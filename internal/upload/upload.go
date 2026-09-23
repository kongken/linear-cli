package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/graphql"
)

const maxFileSize = 100 * 1024 * 1024

var mimeTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml",
	".pdf": "application/pdf", ".txt": "text/plain", ".md": "text/markdown",
	".json": "application/json", ".zip": "application/zip",
	".go": "text/x-go", ".ts": "text/typescript", ".js": "text/javascript",
}

var publicTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true,
	"image/webp": true, "image/bmp": true, "image/tiff": true,
}

// Result is a successful file upload.
type Result struct {
	AssetURL    string
	Filename    string
	Size        int64
	ContentType string
	Public      bool
}

// File uploads a local file to Linear cloud storage and returns the asset URL.
func File(ctx context.Context, path string, makePublic bool) (*Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NewValidationError(fmt.Sprintf("File not found: %s", path))
		}
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.NewValidationError(fmt.Sprintf("Not a file: %s", path),
			errors.WithSuggestion("Please provide a path to a valid file"))
	}
	if info.Size() > maxFileSize {
		return nil, errors.NewValidationError(
			fmt.Sprintf("File too large: %.2fMB exceeds limit of 100MB", float64(info.Size())/(1024*1024)),
			errors.WithSuggestion("Please upload a file smaller than 100MB"),
		)
	}

	filename := filepath.Base(path)
	contentType := mimeTypes[strings.ToLower(filepath.Ext(path))]
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if makePublic && !publicTypes[contentType] {
		return nil, errors.NewValidationError(
			fmt.Sprintf("Cannot upload %s to a public URL", contentType),
			errors.WithSuggestion("Linear only allows public uploads for raster images. Remove --public to upload privately."),
		)
	}

	client, err := graphql.NewClient()
	if err != nil {
		return nil, err
	}
	data, err := client.RequestRaw(ctx, `
mutation FileUpload($contentType: String!, $filename: String!, $size: Int!, $makePublic: Boolean) {
  fileUpload(contentType: $contentType, filename: $filename, size: $size, makePublic: $makePublic) {
    success
    uploadFile {
      assetUrl
      uploadUrl
      headers { key value }
    }
  }
}`, map[string]any{
		"contentType": contentType,
		"filename":    filename,
		"size":        int(info.Size()),
		"makePublic":  makePublic,
	})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		FileUpload struct {
			Success    bool `json:"success"`
			UploadFile *struct {
				AssetURL  string `json:"assetUrl"`
				UploadURL string `json:"uploadUrl"`
				Headers   []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"uploadFile"`
		} `json:"fileUpload"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	if !parsed.FileUpload.Success || parsed.FileUpload.UploadFile == nil {
		return nil, errors.NewCliError("Failed to get upload URL from Linear")
	}
	uf := parsed.FileUpload.UploadFile

	fileData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uf.UploadURL, bytes.NewReader(fileData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	for _, h := range uf.Headers {
		req.Header.Set(h.Key, h.Value)
	}
	httpClient := &http.Client{Timeout: 5 * time.Minute}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.NewCliError(fmt.Sprintf("Upload failed with status %d: %s", resp.StatusCode, string(body)))
	}
	return &Result{
		AssetURL:    uf.AssetURL,
		Filename:    filename,
		Size:        info.Size(),
		ContentType: contentType,
		Public:      makePublic,
	}, nil
}
