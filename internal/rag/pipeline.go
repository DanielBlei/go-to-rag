package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

// Embedder is the minimal interface Retrieve needs from the ollama client.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Retrieve embeds the query, searches the store, and returns the top results
// joined as a single string. Returns "" if no results are found.
func Retrieve(ctx context.Context, query string, limit int, client Embedder, store vectorstore.Store) (string, error) {
	vec, err := client.Embed(ctx, query)
	if err != nil {
		return "", fmt.Errorf("embed query: %w", err)
	}

	results, err := store.Search(ctx, vec, limit)
	if err != nil {
		return "", fmt.Errorf("search store: %w", err)
	}

	if len(results) == 0 {
		return "", nil
	}

	texts := make([]string, len(results))
	for i, r := range results {
		texts[i] = r.Text
	}
	return strings.Join(texts, "\n---\n"), nil
}
