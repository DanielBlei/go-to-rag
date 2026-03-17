package seed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:  "valid manifest",
			input: "docs:\n  - url: https://example.com/a.md\n    name: a.md\n  - url: https://example.com/b.md\n    name: b.md\n",
			want:  2,
		},
		{
			name:  "empty docs",
			input: "docs: []\n",
			want:  0,
		},
		{
			name:    "invalid yaml",
			input:   ":\n  :\n  - :\n[",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseManifest([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseManifest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && len(m.Docs) != tt.want {
				t.Errorf("got %d docs, want %d", len(m.Docs), tt.want)
			}
		})
	}
}

func TestDefaultManifest(t *testing.T) {
	m, err := DefaultManifest()
	if err != nil {
		t.Fatalf("DefaultManifest() error = %v", err)
	}
	if len(m.Docs) == 0 {
		t.Fatal("DefaultManifest() returned 0 docs")
	}
	for _, doc := range m.Docs {
		if doc.URL == "" {
			t.Error("doc has empty URL")
		}
		if doc.Name == "" {
			t.Error("doc has empty Name")
		}
	}
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "test.yaml")
	data := []byte("docs:\n  - url: https://example.com/doc.md\n    name: doc.md\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if len(m.Docs) != 1 {
		t.Fatalf("got %d docs, want 1", len(m.Docs))
	}

	_, err = LoadManifest(filepath.Join(dir, "nonexistent.yaml"))
	if err == nil {
		t.Fatal("LoadManifest() expected error for missing file")
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"raw githubusercontent", "https://raw.githubusercontent.com/org/repo/main/file.md", false},
		{"non-github URL", "https://example.com/docs/page.md", false},
		{"github blob URL", "https://github.com/org/repo/blob/main/file.md", true},
		{"github tree URL", "https://github.com/org/repo/tree/main/docs", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestDownloadRejectsGitHubBlobURL(t *testing.T) {
	dir := t.TempDir()
	doc := Doc{URL: "https://github.com/org/repo/blob/main/file.md", Name: "file.md"}
	_, err := Download(context.Background(), doc, dir)
	if err == nil {
		t.Fatal("Download() expected error for github.com blob URL")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "file.md")); statErr == nil {
		t.Fatal("file should not have been created")
	}
}

func TestDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.md":
			_, _ = fmt.Fprint(w, "# Hello")
		case "/fail.md":
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	ctx := context.Background()

	// Successful download.
	doc := Doc{URL: srv.URL + "/ok.md", Name: "ok.md"}
	if _, err := Download(ctx, doc, dir); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "ok.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Hello" {
		t.Errorf("got %q, want %q", got, "# Hello")
	}

	// No-clobber: second call should skip (file already exists).
	if downloaded, err := Download(ctx, doc, dir); err != nil || downloaded {
		t.Fatalf("Download() no-clobber: downloaded=%v err=%v", downloaded, err)
	}

	// HTTP error.
	badDoc := Doc{URL: srv.URL + "/fail.md", Name: "fail.md"}
	if _, err := Download(ctx, badDoc, dir); err == nil {
		t.Fatal("Download() expected error for 404")
	}

	// Cancelled context.
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	cancelDoc := Doc{URL: srv.URL + "/ok.md", Name: "cancelled.md"}
	if _, err := Download(cancelCtx, cancelDoc, dir); err == nil {
		t.Fatal("Download() expected error for cancelled context")
	}
}

func TestRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "content of %s", r.URL.Path)
	}))
	defer srv.Close()

	dir := t.TempDir()
	manifest := &Manifest{
		Docs: []Doc{
			{URL: srv.URL + "/a.md", Name: "a.md"},
			{URL: srv.URL + "/b.md", Name: "b.md"},
		},
	}

	// First run: downloads both.
	n, err := Run(context.Background(), manifest, dir)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if n != 2 {
		t.Errorf("Run() wrote %d files, want 2", n)
	}

	// Second run: skips both.
	n, err = Run(context.Background(), manifest, dir)
	if err != nil {
		t.Fatalf("Run() second call error = %v", err)
	}
	if n != 0 {
		t.Errorf("Run() second call wrote %d files, want 0", n)
	}
}
