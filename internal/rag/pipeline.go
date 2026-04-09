package rag

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

// FallbackSystemPrompt is used when no retrieved context is available or withFallback is set.
const FallbackSystemPrompt = `You are a helpful assistant with deep knowledge of software and AI systems.
Answer questions clearly and concisely.

Rules:
- Use the provided context as your primary source.
- You may supplement the context with your own knowledge to give a more complete answer.
- When adding information not found in the context, you HAVE TO inform the user:
  "Note: supplementing answer with my own knowledge."
- Never fabricate facts or sources.`

// ThinkMode controls whether and how the model reasons before answering.
type ThinkMode int

const (
	ThinkAuto     ThinkMode = iota // model default
	ThinkDisabled                  // model skips reasoning
	ThinkHidden                    // model reasons; tokens not streamed to caller
)

// ChatOptions carries per-request options forwarded to ChatServer.Chat.
type ChatOptions struct {
	ThinkMode ThinkMode
}

// ChatServer generates LLM responses from context and a prompt.
type ChatServer interface {
	Chat(
		ctx context.Context,
		systemPrompt, contextBlock, userPrompt string,
		opts ChatOptions,
		w io.Writer,
	) error
}

// Embedder is the minimal interface Retrieve needs from the ollama client.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Pipeline is the primary interface for the RAG retrieval pipeline.
type Pipeline interface {
	Retrieve(ctx context.Context, query string, limit int) (string, error)
	RetrieveChunks(ctx context.Context, query string, limit int) ([]vectorstore.Result, error)
}

type pipeline struct {
	embedder Embedder
	store    vectorstore.Store
}

func (p *pipeline) Retrieve(ctx context.Context, query string, limit int) (string, error) {
	return Retrieve(ctx, query, limit, p.embedder, p.store)
}

func (p *pipeline) RetrieveChunks(
	ctx context.Context,
	query string,
	limit int,
) ([]vectorstore.Result, error) {
	return RetrieveChunks(ctx, query, limit, p.embedder, p.store)
}

// NewPipeline returns a Pipeline backed by the given embedder and store.
func NewPipeline(embedder Embedder, store vectorstore.Store) Pipeline {
	return &pipeline{embedder: embedder, store: store}
}

// RetrieveChunks embeds the query, searches the store, and returns structured results.
func RetrieveChunks(
	ctx context.Context,
	query string,
	limit int,
	client Embedder,
	store vectorstore.Store,
) ([]vectorstore.Result, error) {
	vec, err := client.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	results, err := store.Search(ctx, vec, limit)
	if err != nil {
		return nil, fmt.Errorf("search store: %w", err)
	}

	return results, nil
}

// Retrieve embeds the query, searches the store, and returns the top results
// joined as a single string. Returns "" if no results are found.
func Retrieve(
	ctx context.Context,
	query string,
	limit int,
	client Embedder,
	store vectorstore.Store,
) (string, error) {
	results, err := RetrieveChunks(ctx, query, limit, client, store)
	if err != nil {
		return "", err
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

// Ask retrieves context via the pipeline, and streams the LLM answer to w.
// The chat argument is automatically wrapped with newInjectionGuard,
// which frames the context block with untrusted-data sentinels and
// appends an injection-awareness notice to the system prompt.
// Callers do not need to apply injection guarding themselves.
// Returns the retrieved context block so callers can inspect it (e.g. to log empty results).
func Ask(
	ctx context.Context,
	retriever Pipeline,
	chat ChatServer,
	question string,
	topK int,
	withFallback bool,
	opts ChatOptions,
	w io.Writer,
) (string, error) {
	contextBlock, err := retriever.Retrieve(ctx, question, topK)
	if err != nil {
		return "", fmt.Errorf("retrieve: %w", err)
	}

	var sysPrompt string
	if withFallback || contextBlock == "" {
		sysPrompt = FallbackSystemPrompt
	}

	if err := newInjectionGuard(
		chat,
	).Chat(ctx, sysPrompt, contextBlock, question, opts, w); err != nil {
		return contextBlock, fmt.Errorf("chat: %w", err)
	}
	return contextBlock, nil
}
