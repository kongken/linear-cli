package display_test

import (
	"strings"
	"testing"

	"github.com/kongken/linear-cli/internal/display"
)

func TestExtractAndRewriteImageURLs(t *testing.T) {
	md := "See ![logo](https://example.com/a.png) and ![x](https://example.com/a.png)."
	urls := display.ExtractImageURLs(md)
	if len(urls) != 1 || urls[0] != "https://example.com/a.png" {
		t.Fatalf("%v", urls)
	}
	rewritten := display.RewriteImageURLs(md, map[string]string{
		"https://example.com/a.png": "/tmp/a.png",
	})
	if !strings.Contains(rewritten, "/tmp/a.png") {
		t.Fatalf("%s", rewritten)
	}
	if strings.Contains(rewritten, "https://example.com/a.png") {
		t.Fatalf("url not rewritten: %s", rewritten)
	}
}
