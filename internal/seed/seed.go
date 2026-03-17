package seed

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed seed_data.yaml
var defaultManifest []byte

// Doc represents a single document to download.
type Doc struct {
	URL  string `yaml:"url"`
	Name string `yaml:"name"`
}

// Manifest holds the list of documents to download.
type Manifest struct {
	Docs []Doc `yaml:"docs"`
}

// ParseManifest parses a YAML manifest from raw bytes.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// LoadManifest reads and parses a manifest from a file path.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return ParseManifest(data)
}

// DefaultManifest returns the built-in manifest embedded in the binary.
func DefaultManifest() (*Manifest, error) {
	return ParseManifest(defaultManifest)
}

// validateURL rejects GitHub blob/tree URLs that return HTML instead of raw content.
func validateURL(url string) error {
	if strings.Contains(url, "github.com/") && (strings.Contains(url, "/blob/") || strings.Contains(url, "/tree/")) {
		return fmt.Errorf("URL %q points to a GitHub HTML page, use raw.githubusercontent.com instead", url)
	}
	return nil
}

// Download fetches a single document and writes it to destDir.
// Returns true if the file was downloaded, false if it already existed.
func Download(ctx context.Context, doc Doc, destDir string) (bool, error) {
	if err := validateURL(doc.URL); err != nil {
		return false, fmt.Errorf("validate %s: %w", doc.Name, err)
	}

	destPath := filepath.Join(destDir, doc.Name)
	if _, err := os.Stat(destPath); err == nil {
		return false, nil // already exists, skip
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, doc.URL, nil)
	if err != nil {
		return false, fmt.Errorf("create request for %s: %w", doc.Name, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", doc.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("download %s: HTTP %d", doc.Name, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("read response for %s: %w", doc.Name, err)
	}

	if err := os.WriteFile(destPath, body, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", destPath, err)
	}

	return true, nil
}

// Run downloads all documents in the manifest to destDir.
// Returns the number of files written.
func Run(ctx context.Context, manifest *Manifest, destDir string) (int, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return 0, fmt.Errorf("create seed directory: %w", err)
	}

	var written int
	for _, doc := range manifest.Docs {
		downloaded, err := Download(ctx, doc, destDir)
		if err != nil {
			return written, fmt.Errorf("seed: %w", err)
		}
		if downloaded {
			written++
		}
	}

	return written, nil
}
