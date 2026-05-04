package inference

import (
	"context"
	"fmt"

	"github.com/DanielBlei/go-to-rag/internal/ollama"
	"github.com/DanielBlei/go-to-rag/internal/rag"
	"github.com/DanielBlei/go-to-rag/internal/vllm"
)

// ResolveConfig holds all parameters for Resolve.
type ResolveConfig struct {
	Provider   string // "ollama" or "vllm"
	ChatHost   string // chat/inference server URL
	EmbedHost  string // embedding server URL; defaults to ChatHost when empty
	EmbedModel string
	ChatModel  string // empty for embed-only commands (ingest, eval)
	APIKey     string // optional bearer token
	CheckEmbed bool
	CheckChat  bool
}

// Resolve constructs and validates the appropriate backend client.
// cfg.ChatModel may be empty for embed-only commands (ingest, eval);
// in that case the returned ChatServer is nil.
func Resolve(ctx context.Context, cfg ResolveConfig) (rag.Embedder, rag.ChatServer, error) {
	switch cfg.Provider {
	case "ollama":
		c, err := ollama.New(cfg.ChatHost, cfg.EmbedHost, cfg.EmbedModel, cfg.ChatModel, cfg.APIKey)
		if err != nil {
			return nil, nil, fmt.Errorf("backend init: %w", err)
		}
		if err := c.Validate(ctx, cfg.CheckEmbed, cfg.CheckChat); err != nil {
			return nil, nil, fmt.Errorf("backend validation: %w", err)
		}
		var chatServer rag.ChatServer
		if cfg.ChatModel != "" {
			chatServer = c
		}
		return c, chatServer, nil

	case "vllm":
		c, err := vllm.New(cfg.ChatHost, cfg.EmbedHost, cfg.EmbedModel, cfg.ChatModel, cfg.APIKey)
		if err != nil {
			return nil, nil, fmt.Errorf("backend init: %w", err)
		}
		if err := c.Validate(ctx, cfg.CheckEmbed, cfg.CheckChat); err != nil {
			return nil, nil, fmt.Errorf("backend validation: %w", err)
		}
		var chatServer rag.ChatServer
		if cfg.ChatModel != "" {
			chatServer = c
		}
		return c, chatServer, nil

	default:
		return nil, nil, fmt.Errorf("unknown inference provider %q, must be ollama or vllm", cfg.Provider)
	}
}
