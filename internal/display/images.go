package display

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	linearconst "github.com/kongken/linear-cli/internal/const"
)

var mdImageRE = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^)\s]+)\)`)

// ExtractImageURLs returns remote image URLs referenced in markdown.
func ExtractImageURLs(md string) []string {
	matches := mdImageRE.FindAllStringSubmatch(md, -1)
	seen := map[string]bool{}
	urls := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		u := m[1]
		if seen[u] {
			continue
		}
		seen[u] = true
		urls = append(urls, u)
	}
	return urls
}

// DownloadImages downloads remote markdown images into a temp cache and returns
// a map of original URL → local file path. Failures for individual URLs are skipped.
func DownloadImages(md string, headers map[string]string) (map[string]string, error) {
	urls := ExtractImageURLs(md)
	if len(urls) == 0 {
		return map[string]string{}, nil
	}
	dir := filepath.Join(os.TempDir(), "linear-cli-images")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	out := map[string]string{}
	for _, u := range urls {
		sum := sha256.Sum256([]byte(u))
		name := hex.EncodeToString(sum[:16])
		ext := filepath.Ext(strings.Split(u, "?")[0])
		if ext == "" || len(ext) > 5 {
			ext = ".img"
		}
		path := filepath.Join(dir, name+ext)
		if _, err := os.Stat(path); err == nil {
			out[u] = path
			continue
		}
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		if host, err := urlHost(u); err == nil && host == linearconst.PrivateUploadHost {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			continue
		}
		out[u] = path
	}
	return out, nil
}

// RewriteImageURLs replaces markdown image URLs using the provided map.
func RewriteImageURLs(md string, urlToPath map[string]string) string {
	if len(urlToPath) == 0 {
		return md
	}
	return mdImageRE.ReplaceAllStringFunc(md, func(match string) string {
		sub := mdImageRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		if local, ok := urlToPath[sub[1]]; ok {
			return strings.Replace(match, sub[1], local, 1)
		}
		return match
	})
}

// PrepareMarkdown optionally downloads images and rewrites URLs for local viewing.
func PrepareMarkdown(md string, download bool, headers map[string]string) (string, error) {
	if !download || md == "" {
		return md, nil
	}
	mapping, err := DownloadImages(md, headers)
	if err != nil {
		return md, fmt.Errorf("download images: %w", err)
	}
	return RewriteImageURLs(md, mapping), nil
}

func urlHost(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}
