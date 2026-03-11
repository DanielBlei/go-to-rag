package ollama

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ollama/ollama/api"
)

const (
	defaultTimeout = 30 * time.Second
	chatTimeout    = 3 * time.Minute
)

// Client wraps the Ollama API for embedding and chat generation.
type Client struct {
	api        *api.Client
	embedModel string
	chatModel  string
}

// New creates a Client connected to the given host using the specified models.
func New(ctx context.Context, host, embedModel, chatModel string) (*Client, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("invalid ollama host %q: %w", host, err)
	}
	c := &Client{
		api:        api.NewClient(u, http.DefaultClient),
		embedModel: embedModel,
		chatModel:  chatModel,
	}
	if err := c.checkModels(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// checkModels verifies that the required models are available locally.
func (c *Client) checkModels(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	resp, err := c.api.List(ctx)
	if err != nil {
		return fmt.Errorf("cannot reach ollama at, is it running? (%w)", err)
	}
	pulled := make(map[string]bool, len(resp.Models))
	for _, m := range resp.Models {
		pulled[m.Model] = true
	}
	// embedModel is intentionally excluded, not yet implemented.
	for _, model := range []string{c.chatModel} {
		if !pulled[model] && !pulled[model+":latest"] {
			return fmt.Errorf("model %q not found\n  check installed models: `ollama list`\n  to pull it: `ollama pull %s`", model, model)
		}
	}
	return nil
}

// Embed returns the embedding vector for the given text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	resp, err := c.api.Embed(ctx, &api.EmbedRequest{
		Model: c.embedModel,
		Input: text,
	})
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("embed: empty response from ollama")
	}
	return resp.Embeddings[0], nil
}

// Chat sends a single-turn prompt, streaming each token to w as it arrives.
func (c *Client) Chat(ctx context.Context, prompt string, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()
	req := &api.ChatRequest{
		Model: c.chatModel,
		Messages: []api.Message{
			{Role: "user", Content: prompt},
		},
	}
	err := c.api.Chat(ctx, req, func(resp api.ChatResponse) error {
		_, err := fmt.Fprint(w, resp.Message.Content)
		return err
	})
	if err != nil {
		return fmt.Errorf("chat: %w", err)
	}
	_, err = fmt.Fprintln(w)
	return err
}
