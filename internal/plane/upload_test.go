package plane_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kongken/linear-cli/internal/plane"
	"github.com/kongken/linear-cli/internal/planeconfig"
)

func TestAttachmentUploadSeparatesAPIAndStorageCredentials(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			var stage atomic.Int32
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" {
					t.Error("Plane credential leaked to storage")
				}
				if !stage.CompareAndSwap(1, 2) {
					t.Error("upload order changed")
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					return
				}
				file, _, err := r.FormFile("file")
				if err != nil {
					t.Error(err)
					return
				}
				defer file.Close()
				data, err := io.ReadAll(file)
				if err != nil || string(data) != "attachment content" || r.FormValue("policy") != "signed-policy" {
					t.Errorf("storage form: %q %v", data, err)
				}
				w.WriteHeader(204)
			}))
			defer storage.Close()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-API-Key") != "private-plane-token" {
					t.Error("missing API credential")
				}
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" {
					if !stage.CompareAndSwap(0, 1) {
						t.Error("allocation order changed")
					}
					response := struct {
						AssetID string `json:"asset_id"`
						Upload  struct {
							URL    string            `json:"url"`
							Fields map[string]string `json:"fields"`
						} `json:"upload_data"`
					}{AssetID: "asset"}
					response.Upload.URL = storage.URL
					response.Upload.Fields = map[string]string{"policy": "signed-policy"}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						t.Error(err)
					}
				} else if r.Method == "PATCH" {
					if !stage.CompareAndSwap(2, 3) {
						t.Error("confirmed before storage accepted file")
					}
					var body struct {
						Uploaded bool `json:"is_uploaded"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !body.Uploaded {
						t.Error("confirmation was not true")
					}
					if version == "v1" {
						w.WriteHeader(204)
					} else {
						_, _ = w.Write([]byte(`{"id":"asset","is_uploaded":true}`))
					}
				} else {
					_, _ = w.Write([]byte(`{"id":"asset","is_uploaded":true}`))
				}
			}))
			defer api.Close()
			path := filepath.Join(t.TempDir(), "notes.txt")
			if err := os.WriteFile(path, []byte("attachment content"), 0600); err != nil {
				t.Fatal(err)
			}
			c, err := plane.NewClient(planeconfig.Context{Profile: planeconfig.Profile{BaseURL: api.URL, Workspace: "acme", APIVersion: version, AllowHTTP: true}}, "private-plane-token", plane.ClientOptions{})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := c.UploadAttachment(context.Background(), plane.IssueRef{ID: "issue", ProjectID: "project"}, path, "")
			if err != nil || stage.Load() != 3 || !strings.Contains(string(raw), `"is_uploaded":true`) {
				t.Fatalf("upload: %s stage=%d err=%v", raw, stage.Load(), err)
			}
		})
	}
}
