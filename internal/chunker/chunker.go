package chunker

import (
	"fmt"
	"os"
	"strings"
)

// Chunk is a text segment extracted from a source file.
type Chunk struct {
	Text       string
	Index      int
	SourceFile string
}

// ChunkText splits text into overlapping character-based chunks.
// chunkSize must be > 0; overlap must be >= 0 and < chunkSize.
// Whitespace-only chunks are skipped.
func ChunkText(text, sourceFile string, chunkSize, overlap int) ([]Chunk, error) {
	if chunkSize <= 0 {
		return nil, fmt.Errorf("chunkSize must be > 0")
	}
	if overlap < 0 || overlap >= chunkSize {
		return nil, fmt.Errorf("overlap must be >= 0 and < chunkSize")
	}

	runes := []rune(text)
	step := chunkSize - overlap

	var chunks []Chunk
	for start := 0; start < len(runes); start += step {
		end := min(start+chunkSize, len(runes))
		segment := string(runes[start:end])
		if strings.TrimSpace(segment) == "" {
			continue
		}
		chunks = append(chunks, Chunk{
			Text:       segment,
			Index:      len(chunks),
			SourceFile: sourceFile,
		})
	}
	return chunks, nil
}

// ChunkFile reads a file and calls ChunkText on its contents.
func ChunkFile(path string, chunkSize, overlap int) ([]Chunk, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return ChunkText(string(data), path, chunkSize, overlap)
}
