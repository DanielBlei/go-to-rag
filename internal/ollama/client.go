package ollama

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ollama/ollama/api"
)

const (
	defaultTimeout = 30 * time.Second
	chatTimeout    = 3 * time.Minute
)

// Client wraps the Ollama API for embedding and chat generation.
type Client struct {
	api          *api.Client
	embedModel   string
	chatModel    string
	validateOnce sync.Once
	validateErr  error
}

// New creates a Client connected to the given host using the specified models.
func New(host, embedModel, chatModel string) (*Client, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("invalid host %q: %w", host, err)
	}
	return &Client{
		api:        api.NewClient(u, http.DefaultClient),
		embedModel: embedModel,
		chatModel:  chatModel,
	}, nil
}

// Validate confirms that Ollama is reachable and the required models are available.
// TODO: to be reviewed, flip checkEmbed=true when the vector store is wired up.
func (c *Client) Validate(ctx context.Context, checkEmbed, checkChat bool) error {
	c.validateOnce.Do(func() {
		tctx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()

		resp, err := c.api.List(tctx)
		if err != nil {
			c.validateErr = fmt.Errorf("connect to ollama: %w", err)
			return
		}
		for _, want := range c.wantedModels(checkEmbed, checkChat) {
			if !modelAvailable(resp.Models, want) {
				c.validateErr = fmt.Errorf("model %q not found — run: ollama pull %s", want, want)
				return
			}
		}
	})
	return c.validateErr
}

func (c *Client) wantedModels(checkEmbed, checkChat bool) []string {
	var models []string
	if checkEmbed {
		models = append(models, c.embedModel)
	}
	if checkChat {
		models = append(models, c.chatModel)
	}
	return models
}

func modelAvailable(models []api.ListModelResponse, want string) bool {
	for _, m := range models {
		if m.Model == want || m.Model == want+":latest" {
			return true
		}
	}
	return false
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
