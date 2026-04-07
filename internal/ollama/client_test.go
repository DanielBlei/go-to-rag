package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ollama/ollama/api"

	"github.com/DanielBlei/go-to-rag/internal/rag"
)

const (
	testEmbedModel = "mxbai-embed-large:latest"
	testChatModel  = "llama3.2:1b"
	testThinkModel = "qwen3:1.7b"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		wantErr    bool
		embedModel string
		chatModel  string
	}{
		{
			name:       "valid URL",
			host:       "http://localhost:11434",
			embedModel: testEmbedModel,
			chatModel:  testChatModel,
			wantErr:    false,
		},
		{
			name:       "missing tag on embed model",
			host:       "http://localhost:11434",
			embedModel: "mxbai-embed-large",
			chatModel:  testChatModel,
			wantErr:    true,
		},
		{
			name:       "missing tag on chat model",
			host:       "http://localhost:11434",
			embedModel: testEmbedModel,
			chatModel:  "llama3.2",
			wantErr:    true,
		},
		{
			name:       "empty chat model (ingest use case)",
			host:       "http://localhost:11434",
			embedModel: testEmbedModel,
			chatModel:  "",
			wantErr:    false,
		},
		{
			name:    "invalid URL",
			host:    "://bad-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.host, tt.embedModel, tt.chatModel)
			if (err != nil) != tt.wantErr {
				t.Fatalf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if c.embedModel != tt.embedModel || c.chatModel != tt.chatModel {
					t.Fatal("model fields not set correctly")
				}
			}
		})
	}
}

func TestModelAvailable(t *testing.T) {
	models := []api.ListModelResponse{
		{Model: testChatModel},
		{Model: testEmbedModel},
	}

	tests := []struct {
		want      string
		available bool
	}{
		{testChatModel, true},
		{"mxbai-embed-large", true}, // should match ":latest" suffix
		{testEmbedModel, true},
		{"mistral", false},
	}

	for _, tt := range tests {
		got := modelAvailable(models, tt.want)
		if got != tt.available {
			t.Errorf("modelAvailable(%q) = %v, want %v", tt.want, got, tt.available)
		}
	}
}

// newTestServer spins up an httptest server that responds to GET /api/tags
// with the provided model names in the list response.
func newTestServer(t *testing.T, modelNames []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		var models []api.ListModelResponse
		for _, name := range modelNames {
			models = append(models, api.ListModelResponse{Model: name})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.ListResponse{Models: models})
	}))
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name         string
		serverModels []string
		host         string // overrides server URL if set
		checkChat    bool
		wantErr      bool
		errContains  string
	}{
		{
			name:         "model found",
			serverModels: []string{"llama3.2:1b"},
			checkChat:    true,
			wantErr:      false,
		},
		{
			name:         "model missing",
			serverModels: []string{"some-other-model"},
			checkChat:    true,
			wantErr:      true,
			errContains:  "llama3.2:1b",
		},
		{
			name:      "unreachable host",
			host:      "http://127.0.0.1:1",
			checkChat: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var host string
			if tt.host != "" {
				host = tt.host
			} else {
				srv := newTestServer(t, tt.serverModels)
				defer srv.Close()
				host = srv.URL
			}

			c, err := New(host, testEmbedModel, testChatModel)
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err = c.Validate(ctx, false, tt.checkChat)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error should contain %q, got: %v", tt.errContains, err)
			}
		})
	}
}

// mockThinkingWriter implements ThinkingWriter for testing.
type mockThinkingWriter struct {
	answer   strings.Builder
	thinking strings.Builder
}

func (m *mockThinkingWriter) Write(p []byte) (int, error) {
	return m.answer.Write(p)
}

func (m *mockThinkingWriter) WriteThinking(p []byte) (int, error) {
	return m.thinking.Write(p)
}

// newTestChatServer creates an httptest server that mocks the Ollama /api/chat endpoint.
// It emits thinking tokens only for models that support thinking (qwen3:*).
func newTestChatServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher := w.(http.Flusher)

		// Determine if this model supports thinking by parsing the request.
		var req api.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			// Stream thinking token only if it's a thinking model.
			if strings.Contains(req.Model, "qwen3") {
				thinkResp := api.ChatResponse{Message: api.Message{Thinking: "<think>analyzing</think>"}}
				_ = json.NewEncoder(w).Encode(thinkResp)
				flusher.Flush()
			}
		}

		// Stream answer token.
		answerResp := api.ChatResponse{Message: api.Message{Content: "test answer"}}
		_ = json.NewEncoder(w).Encode(answerResp)
		flusher.Flush()

		// Done sentinel.
		doneResp := api.ChatResponse{Done: true}
		_ = json.NewEncoder(w).Encode(doneResp)
		flusher.Flush()
	}))
}

func TestChat(t *testing.T) {
	tests := []struct {
		name               string
		model              string
		thinkMode          rag.ThinkMode
		wantThinkingRouted bool // for ThinkingWriter
	}{
		{
			name:               "ThinkAuto routes thinking to ThinkingWriter",
			model:              testThinkModel,
			thinkMode:          rag.ThinkAuto,
			wantThinkingRouted: true,
		},
		{
			name:               "ThinkHidden suppresses thinking output",
			model:              testThinkModel,
			thinkMode:          rag.ThinkHidden,
			wantThinkingRouted: false,
		},
		{
			name:               "ThinkDisabled suppresses thinking output",
			model:              testThinkModel,
			thinkMode:          rag.ThinkDisabled,
			wantThinkingRouted: false,
		},
		{
			name:               "non-thinking model with ThinkAuto works",
			model:              testChatModel,
			thinkMode:          rag.ThinkAuto,
			wantThinkingRouted: false, // model doesn't emit thinking, so nothing to route
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestChatServer(t)
			defer srv.Close()

			c, err := New(srv.URL, testEmbedModel, tt.model)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			chatOpts := rag.ChatOptions{ThinkMode: tt.thinkMode}

			// Test ThinkingWriter path.
			mockWriter := &mockThinkingWriter{}
			chatErr := c.Chat(ctx, "system", "", "question", chatOpts, mockWriter)
			if chatErr != nil {
				t.Fatalf("Chat with ThinkingWriter: %v", chatErr)
			}

			// Verify routing based on the test case expectations.
			if tt.wantThinkingRouted {
				// Thinking should be routed to WriteThinking.
				if mockWriter.thinking.Len() == 0 {
					t.Error("expected thinking to be routed, got empty")
				}
			} else {
				// Thinking should be suppressed, not routed.
				if mockWriter.thinking.Len() != 0 {
					t.Errorf("expected thinking to be suppressed, got: %q", mockWriter.thinking.String())
				}
			}

			// All cases should have the answer.
			if mockWriter.answer.Len() == 0 {
				t.Error("expected answer to be routed, got empty")
			}
			if !strings.Contains(mockWriter.answer.String(), "test answer") {
				t.Errorf("expected answer in output, got: %q", mockWriter.answer.String())
			}
		})
	}
}
