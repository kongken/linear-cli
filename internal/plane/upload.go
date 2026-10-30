package plane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxAttachmentSize = 100 * 1024 * 1024

// UploadAttachment performs allocation, signed multipart upload, then confirmation.
// A failed stage reports the allocated asset ID instead of repeating a write.
func (c *Client) UploadAttachment(ctx context.Context, ref IssueRef, path, contentType string) (json.RawMessage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open attachment: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxAttachmentSize {
		return nil, fmt.Errorf("attachment must be a regular file between 1 byte and 100 MiB")
	}
	name := filepath.Base(path)
	if strings.ContainsAny(name, "\r\n") {
		return nil, fmt.Errorf("attachment filename cannot contain line breaks")
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(name))
	}
	if contentType == "" {
		sample := make([]byte, 512)
		n, e := file.Read(sample)
		if e != nil && e != io.EOF {
			return nil, e
		}
		contentType = http.DetectContentType(sample[:n])
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	}
	contentType, _, err = mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("invalid attachment MIME type")
	}
	metadata, err := json.Marshal(struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		Type string `json:"type"`
	}{name, info.Size(), contentType})
	if err != nil {
		return nil, err
	}
	raw, err := c.MutateSubresource(ctx, ref, "attachments", "", http.MethodPost, metadata)
	if err != nil {
		return nil, err
	}
	var upload struct {
		AssetID string `json:"asset_id"`
		Data    struct {
			URL    string            `json:"url"`
			Fields map[string]string `json:"fields"`
		} `json:"upload_data"`
	}
	if err := json.Unmarshal(raw, &upload); err != nil || upload.AssetID == "" {
		return nil, fmt.Errorf("attachment allocation returned an unsupported response; inspect work-item attachments before retrying")
	}
	fail := func(stage string, cause error) (json.RawMessage, error) {
		return nil, fmt.Errorf("attachment %s allocated; %s failed: %w", upload.AssetID, stage, cause)
	}
	if err := segment(upload.AssetID); err != nil {
		return fail("upload metadata", err)
	}
	target, err := url.Parse(upload.Data.URL)
	if err != nil || target.Host == "" || target.User != nil || target.Fragment != "" || (target.Scheme != "https" && !(target.Scheme == "http" && c.Context.AllowHTTP)) || len(upload.Data.Fields) == 0 {
		return fail("storage validation", fmt.Errorf("unsupported signed storage destination"))
	}
	// Construct a streaming multipart body without retaining the file in memory.
	var envelope bytes.Buffer
	writer := multipart.NewWriter(&envelope)
	var keys []string
	for key := range upload.Data.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := writer.WriteField(key, upload.Data.Fields[key]); err != nil {
			return fail("storage form", err)
		}
	}
	if _, err := writer.CreateFormFile("file", name); err != nil {
		return fail("storage form", err)
	}
	prefix := append([]byte(nil), envelope.Bytes()...)
	if err := writer.Close(); err != nil {
		return fail("storage form", err)
	}
	suffix := envelope.Bytes()[len(prefix):]
	body := io.MultiReader(bytes.NewReader(prefix), io.LimitReader(file, info.Size()), bytes.NewReader(suffix))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), body)
	if err != nil {
		return fail("storage request", fmt.Errorf("cannot create storage request"))
	}
	request.ContentLength = int64(len(prefix)) + info.Size() + int64(len(suffix))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	// This request deliberately has no Plane authentication headers.
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return fail("storage upload; result is unknown", ctx.Err())
		}
		return fail("storage upload", fmt.Errorf("transfer failed or was interrupted; inspect the allocated attachment before retrying"))
	}
	_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 65536))
	response.Body.Close()
	if readErr != nil {
		return fail("storage upload", fmt.Errorf("storage response interrupted"))
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fail("storage upload", fmt.Errorf("storage returned HTTP %d", response.StatusCode))
	}
	confirmed, err := c.MutateSubresource(ctx, ref, "attachments", upload.AssetID, http.MethodPatch, json.RawMessage(`{"is_uploaded":true}`))
	if err != nil {
		return fail("confirmation", err)
	}
	if len(confirmed) > 0 {
		return confirmed, nil
	}
	collection, err := c.SubresourcePath(ref, "attachments")
	if err != nil {
		return fail("readback", err)
	}
	confirmed, err = c.RequestRaw(ctx, http.MethodGet, collection+url.PathEscape(upload.AssetID)+"/", nil)
	if err != nil {
		return fail("confirmed metadata readback", err)
	}
	return confirmed, nil
}
