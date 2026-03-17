package chunker

import (
	"os"
	"strings"
	"testing"
)

func TestChunkText_Basic(t *testing.T) {
	// 10 chars, chunkSize=4, overlap=1 → step=3 → starts: 0,3,6,9
	text := "0123456789"
	chunks, err := ChunkText(text, "f.md", 4, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// starts: 0→"0123", 3→"3456", 6→"6789", 9→"9"
	if len(chunks) != 4 {
		t.Fatalf("expected 4 chunks, got %d", len(chunks))
	}
	if chunks[0].Text != "0123" {
		t.Errorf("chunk 0: %q", chunks[0].Text)
	}
	if chunks[1].Text != "3456" {
		t.Errorf("chunk 1: %q", chunks[1].Text)
	}
	if chunks[2].Text != "6789" {
		t.Errorf("chunk 2: %q", chunks[2].Text)
	}
	if chunks[3].Text != "9" {
		t.Errorf("chunk 3: %q", chunks[3].Text)
	}
}

func TestChunkText_Indices(t *testing.T) {
	chunks, _ := ChunkText("abcdefghij", "f.md", 4, 0)
	for i, c := range chunks {
		if c.Index != i {
			t.Errorf("chunk %d has Index %d", i, c.Index)
		}
		if c.SourceFile != "f.md" {
			t.Errorf("chunk %d has SourceFile %q", i, c.SourceFile)
		}
	}
}

func TestChunkText_Overlap(t *testing.T) {
	// chunkSize=6, overlap=2 → step=4
	text := "ABCDEFGHIJ"
	chunks, err := ChunkText(text, "f.md", 6, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// chunk 0: "ABCDEF", chunk 1: "EFGHIJ"
	// Tail of chunk N should match head of chunk N+1 by `overlap` chars.
	for i := 0; i+1 < len(chunks); i++ {
		tail := chunks[i].Text[len(chunks[i].Text)-2:]
		head := chunks[i+1].Text[:2]
		if tail != head {
			t.Errorf("overlap mismatch between chunk %d and %d: %q vs %q", i, i+1, tail, head)
		}
	}
}

func TestChunkText_SingleChunk(t *testing.T) {
	chunks, err := ChunkText("hi", "f.md", 100, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Text != "hi" {
		t.Errorf("expected single chunk 'hi', got %v", chunks)
	}
}

func TestChunkText_EmptyInput(t *testing.T) {
	chunks, err := ChunkText("", "f.md", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestChunkText_WhitespaceOnlySkipped(t *testing.T) {
	chunks, err := ChunkText("   \n\t  ", "f.md", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for whitespace-only input, got %d", len(chunks))
	}
}

func TestChunkText_InvalidParams(t *testing.T) {
	if _, err := ChunkText("abc", "f.md", 0, 0); err == nil {
		t.Error("expected error for chunkSize=0")
	}
	if _, err := ChunkText("abc", "f.md", -1, 0); err == nil {
		t.Error("expected error for chunkSize=-1")
	}
	if _, err := ChunkText("abc", "f.md", 4, -1); err == nil {
		t.Error("expected error for overlap=-1")
	}
	if _, err := ChunkText("abc", "f.md", 4, 4); err == nil {
		t.Error("expected error for overlap==chunkSize")
	}
	if _, err := ChunkText("abc", "f.md", 4, 5); err == nil {
		t.Error("expected error for overlap>chunkSize")
	}
}

func TestChunkFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "chunk*.md")
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("x", 20)
	_, _ = f.WriteString(content)
	_ = f.Close()

	chunks, err := ChunkFile(f.Name(), 8, 0)
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}
	for _, c := range chunks {
		if c.SourceFile != f.Name() {
			t.Errorf("SourceFile = %q, want %q", c.SourceFile, f.Name())
		}
	}
}

func TestChunkFile_Missing(t *testing.T) {
	_, err := ChunkFile("/nonexistent/path/file.md", 10, 0)
	if err == nil {
		t.Error("expected error for missing file")
	}
}
