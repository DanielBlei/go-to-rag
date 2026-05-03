package inference

import (
	"context"
	"fmt"

	"github.com/DanielBlei/go-to-rag/internal/ollama"
	"github.com/DanielBlei/go-to-rag/internal/rag"
	"github.com/DanielBlei/go-to-rag/internal/vllm"
)

// Resolve constructs and validates the appropriate backend client.
// provider must be "ollama" or "vllm". chatModel may be empty for embed-only
// commands (ingest, eval); in that case the returned ChatServer is nil.
func Resolve(
	ctx context.Context,
	provider, host, embedModel, chatModel, apiKey string,
	checkEmbed, checkChat bool,
) (rag.Embedder, rag.ChatServer, error) {
	switch provider {
	case "ollama":
		c, err := ollama.New(host, embedModel, chatModel, apiKey)
		if err != nil {
			return nil, nil, fmt.Errorf("backend init: %w", err)
		}
		if err := c.Validate(ctx, checkEmbed, checkChat); err != nil {
			return nil, nil, fmt.Errorf("backend validation: %w", err)
		}
		var chatServer rag.ChatServer
		if chatModel != "" {
			chatServer = c
		}
		return c, chatServer, nil

	case "vllm":
		c, err := vllm.New(host, embedModel, chatModel, apiKey)
		if err != nil {
			return nil, nil, fmt.Errorf("backend init: %w", err)
		}
		if err := c.Validate(ctx, checkEmbed, checkChat); err != nil {
			return nil, nil, fmt.Errorf("backend validation: %w", err)
		}
		var chatServer rag.ChatServer
		if chatModel != "" {
			chatServer = c
		}
		return c, chatServer, nil

	default:
		return nil, nil, fmt.Errorf("unknown inference provider %q, must be ollama or vllm", provider)
	}
}
