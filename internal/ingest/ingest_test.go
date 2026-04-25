package ingest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

type fakeStore struct {
	mu      sync.Mutex
	chunks  []vectorstore.Result
	deleted []string
	hasErr  error
	addErr  error
	delErr  error
}

func (f *fakeStore) AddChunk(_ context.Context, source, text string, _ []float32, idx int) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chunks = append(f.chunks, vectorstore.Result{Source: source, Text: text, ChunkIndex: idx})
	return nil
}

func (f *fakeStore) Search(context.Context, []float32, int) ([]vectorstore.Result, error) {
	return nil, nil
}

func (f *fakeStore) CountChunks(context.Context) (int, error) { return len(f.chunks), nil }

func (f *fakeStore) HasSource(_ context.Context, source string) (bool, error) {
	if f.hasErr != nil {
		return false, f.hasErr
	}
	for _, c := range f.chunks {
		if c.Source == source {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) DeleteSource(_ context.Context, source string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, source)
	kept := f.chunks[:0]
	for _, c := range f.chunks {
		if c.Source != source {
			kept = append(kept, c)
		}
	}
	f.chunks = kept
	return f.delErr
}

func (f *fakeStore) Close() error { return nil }

type fakeEmbedder struct {
	failOn string
}

func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if f.failOn != "" && len(text) > 0 && text[:1] == f.failOn {
		return nil, errors.New("boom")
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

func writeCorpus(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return dir
}

func relSourcePath(root string) func(string) (string, error) {
	return func(p string) (string, error) {
		return filepath.Rel(root, p)
	}
}

func TestRun_HappyPath(t *testing.T) {
	root := writeCorpus(t, map[string]string{
		"a.md":     "hello world",
		"b.md":     "another doc",
		"sub/c.md": "nested doc",
	})
	store := &fakeStore{}
	srcs, total, err := Run(context.Background(), store, &fakeEmbedder{}, root, Options{
		ChunkSize:  64,
		Overlap:    0,
		Glob:       "*.md",
		SourcePath: relSourcePath(root),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(srcs) != 3 {
		t.Fatalf("expected 3 sources, got %d (%v)", len(srcs), srcs)
	}
	if total < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", total)
	}
}

func TestRun_NoRecursive(t *testing.T) {
	root := writeCorpus(t, map[string]string{
		"a.md":     "top",
		"sub/b.md": "nested",
	})
	store := &fakeStore{}
	srcs, _, err := Run(context.Background(), store, &fakeEmbedder{}, root, Options{
		ChunkSize:   64,
		Glob:        "*.md",
		NoRecursive: true,
		SourcePath:  relSourcePath(root),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(srcs) != 1 || srcs[0] != "a.md" {
		t.Fatalf("expected only a.md, got %v", srcs)
	}
}

func TestRun_SkipExisting(t *testing.T) {
	root := writeCorpus(t, map[string]string{"a.md": "hi"})
	store := &fakeStore{
		chunks: []vectorstore.Result{{Source: "a.md", Text: "hi", ChunkIndex: 0}},
	}
	var skipped int
	srcs, _, err := Run(context.Background(), store, &fakeEmbedder{}, root, Options{
		ChunkSize:    64,
		SkipExisting: true,
		SourcePath:   relSourcePath(root),
		OnFile: func(_ string, _ int, sk bool) {
			if sk {
				skipped++
			}
		},
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(srcs) != 0 {
		t.Fatalf("expected 0 newly ingested, got %v", srcs)
	}
	if skipped != 1 {
		t.Fatalf("expected 1 skip callback, got %d", skipped)
	}
}

func TestRun_RollbackOnEmbedError(t *testing.T) {
	root := writeCorpus(t, map[string]string{
		"a.md": "hello",
		"b.md": "broken doc",
	})
	store := &fakeStore{}
	_, _, err := Run(context.Background(), store, &fakeEmbedder{failOn: "b"}, root, Options{
		ChunkSize:  64,
		SourcePath: relSourcePath(root),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	for _, c := range store.chunks {
		if c.Source == "b.md" {
			t.Fatalf("b.md should have been rolled back")
		}
	}
	found := false
	for _, d := range store.deleted {
		if d == "b.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DeleteSource(b.md), got %v", store.deleted)
	}
}

func TestRun_ContextCancel(t *testing.T) {
	root := writeCorpus(t, map[string]string{
		"a.md": "x",
		"b.md": "y",
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := Run(ctx, &fakeStore{}, &fakeEmbedder{}, root, Options{
		ChunkSize:  64,
		SourcePath: relSourcePath(root),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRun_RequiresSourcePath(t *testing.T) {
	_, _, err := Run(context.Background(), &fakeStore{}, &fakeEmbedder{}, t.TempDir(), Options{
		ChunkSize: 64,
	})
	if err == nil {
		t.Fatalf("expected error when SourcePath is nil")
	}
}

func TestRun_DefaultGlob(t *testing.T) {
	root := writeCorpus(t, map[string]string{
		"a.md":  "x",
		"b.txt": "y",
	})
	store := &fakeStore{}
	srcs, _, err := Run(context.Background(), store, &fakeEmbedder{}, root, Options{
		ChunkSize:  64,
		SourcePath: relSourcePath(root),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(srcs) != 1 || srcs[0] != "a.md" {
		t.Fatalf("default glob should match only *.md, got %v", srcs)
	}
}
