package vllm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DanielBlei/go-to-rag/internal/httpclient"
	"github.com/DanielBlei/go-to-rag/internal/rag"
)

const chatTimeout = 3 * time.Minute

// Client is a minimal OpenAI-compatible HTTP client for vLLM.
// It implements rag.Embedder and rag.ChatServer using stdlib only.
type Client struct {
	host        string
	embedModel  string
	chatModel   string
	httpClient  *http.Client
	once        sync.Once
	validateErr error
}

// New creates a Client connected to the given host. apiKey is optional.
// Unlike the Ollama client, model names are not required to have a tag (e.g.
// "meta-llama/Llama-3.1-8B-Instruct" is a valid vLLM model ID).
func New(host, embedModel, chatModel, apiKey string) (*Client, error) {
	if _, err := url.Parse(host); err != nil {
		return nil, fmt.Errorf("invalid host %q: %w", host, err)
	}
	return &Client{
		host:       strings.TrimRight(host, "/"),
		embedModel: embedModel,
		chatModel:  chatModel,
		httpClient: &http.Client{
			Transport: &httpclient.BearerTransport{Token: apiKey, Base: http.DefaultTransport},
		},
	}, nil
}

// Validate confirms that the vLLM server is reachable and the required models
// are loaded. The check is idempotent — subsequent calls return the cached result.
func (c *Client) Validate(ctx context.Context, checkEmbed, checkChat bool) error {
	c.once.Do(func() {
		validateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(validateCtx, http.MethodGet, c.host+"/v1/models", nil)
		if err != nil {
			c.validateErr = fmt.Errorf("build models request: %w", err)
			return
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.validateErr = fmt.Errorf("connect to vLLM: %w", err)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			c.validateErr = fmt.Errorf("vLLM /v1/models returned %d", resp.StatusCode)
			return
		}

		var modelsResp struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
			c.validateErr = fmt.Errorf("decode models response: %w", err)
			return
		}

		loaded := make(map[string]bool, len(modelsResp.Data))
		for _, m := range modelsResp.Data {
			loaded[m.ID] = true
		}

		if checkEmbed && c.embedModel != "" && !loaded[c.embedModel] {
			c.validateErr = fmt.Errorf("model %q not found in vLLM", c.embedModel)
			return
		}
		if checkChat && c.chatModel != "" && !loaded[c.chatModel] {
			c.validateErr = fmt.Errorf("model %q not found in vLLM", c.chatModel)
		}
	})
	return c.validateErr
}

// Embed returns the embedding vector for the given text using /v1/embeddings.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody, err := json.Marshal(map[string]string{
		"model": c.embedModel,
		"input": text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/v1/embeddings",
		bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed: vLLM returned %d", resp.StatusCode)
	}

	var embedResp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	if len(embedResp.Data) == 0 || len(embedResp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embed: empty response from vLLM")
	}
	return embedResp.Data[0].Embedding, nil
}

// chatRequest is the JSON body sent to /v1/chat/completions.
type chatRequest struct {
	Model            string        `json:"model"`
	Messages         []chatMessage `json:"messages"`
	Stream           bool          `json:"stream"`
	IncludeReasoning *bool         `json:"include_reasoning,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatChunk is a single SSE data frame from the streaming response.
type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
		} `json:"delta"`
	} `json:"choices"`
}

// Chat sends a single-turn prompt to /v1/chat/completions and streams the
// response to w. Thinking tokens (delta.reasoning) are routed via
// rag.ThinkingWriter when the writer implements it.
func (c *Client) Chat(
	ctx context.Context,
	systemPrompt, contextBlock, userPrompt string,
	opts rag.ChatOptions,
	w io.Writer,
) error {
	ctx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()

	messages := buildMessages(systemPrompt, contextBlock, userPrompt)
	body := chatRequest{
		Model:    c.chatModel,
		Messages: messages,
		Stream:   true,
	}
	if opts.ThinkMode == rag.ThinkDisabled {
		f := false
		body.IncludeReasoning = &f
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/v1/chat/completions",
		bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("build chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chat: vLLM returned %d", resp.StatusCode)
	}

	tw, hasTW := w.(rag.ThinkingWriter)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil || len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		if delta.Reasoning != "" {
			if opts.ThinkMode == rag.ThinkAuto && hasTW {
				if _, err := tw.WriteThinking([]byte(delta.Reasoning)); err != nil {
					return err
				}
			}
			// ThinkHidden and ThinkDisabled: discard silently.
			continue
		}
		if delta.Content != "" {
			if _, err := fmt.Fprint(w, delta.Content); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}
	return nil
}

// buildMessages assembles the messages slice for a chat request.
func buildMessages(systemPrompt, contextBlock, userPrompt string) []chatMessage {
	var messages []chatMessage
	if systemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	}
	userContent := userPrompt
	if contextBlock != "" {
		userContent = "Context:\n" + contextBlock + "\n\nQuestion: " + userPrompt
	}
	messages = append(messages, chatMessage{Role: "user", Content: userContent})
	return messages
}
