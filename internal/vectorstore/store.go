package vectorstore

import "context"

// Result is a chunk returned from a similarity search.
type Result struct {
	Source     string
	Text       string
	ChunkIndex int
	Score      float64
}

// Store is the interface for a vector store backend.
type Store interface {
	AddChunk(ctx context.Context, source, text string, embedding []float32, chunkIndex int) error
	Search(ctx context.Context, queryVec []float32, limit int) ([]Result, error)
	CountChunks(ctx context.Context) (int, error)
	HasSource(ctx context.Context, source string) (bool, error)
	DeleteSource(ctx context.Context, source string) error
	Close() error
}
