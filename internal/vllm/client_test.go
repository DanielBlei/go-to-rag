package vllm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DanielBlei/go-to-rag/internal/rag"
)

const (
	testEmbedModel = "BAAI/bge-large-en-v1.5"
	testChatModel  = "meta-llama/Llama-3.1-8B-Instruct"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{name: "valid URL", host: "http://localhost:8000", wantErr: false},
		{name: "invalid URL", host: "://bad", wantErr: true},
		{name: "URL with trailing slash", host: "http://localhost:8000/", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.host, testEmbedModel, testChatModel, "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func newModelsServer(t *testing.T, modelIDs []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		type modelData struct {
			ID string `json:"id"`
		}
		type modelsResp struct {
			Data []modelData `json:"data"`
		}
		resp := modelsResp{}
		for _, id := range modelIDs {
			resp.Data = append(resp.Data, modelData{ID: id})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		serverIDs   []string
		host        string // overrides server URL
		checkEmbed  bool
		checkChat   bool
		wantErr     bool
		errContains string
	}{
		{
			name:       "both models found",
			serverIDs:  []string{testEmbedModel, testChatModel},
			checkEmbed: true, checkChat: true,
			wantErr: false,
		},
		{
			name:        "chat model missing",
			serverIDs:   []string{testEmbedModel},
			checkEmbed:  true,
			checkChat:   true,
			wantErr:     true,
			errContains: testChatModel,
		},
		{
			name:        "embed model missing",
			serverIDs:   []string{testChatModel},
			checkEmbed:  true,
			checkChat:   false,
			wantErr:     true,
			errContains: testEmbedModel,
		},
		{
			name:       "unreachable host",
			host:       "http://127.0.0.1:1",
			checkEmbed: true, checkChat: true,
			wantErr: true,
		},
		{
			name:       "no check skips validation",
			serverIDs:  []string{},
			checkEmbed: false, checkChat: false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var targetHost string
			if tt.host != "" {
				targetHost = tt.host
			} else {
				srv := newModelsServer(t, tt.serverIDs)
				defer srv.Close()
				targetHost = srv.URL
			}

			c, err := New(targetHost, testEmbedModel, testChatModel, "")
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err = c.Validate(ctx, tt.checkEmbed, tt.checkChat)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error should contain %q, got: %v", tt.errContains, err)
			}
		})
	}
}

func TestValidate_OnceGuard(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "intentional error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, testEmbedModel, testChatModel, "")
	ctx := context.Background()
	_ = c.Validate(ctx, true, true)
	_ = c.Validate(ctx, true, true)

	if calls != 1 {
		t.Errorf("Validate called server %d times, want 1 (sync.Once guard)", calls)
	}
}

func newEmbedServer(t *testing.T, embedding []float32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		type embedData struct {
			Embedding []float32 `json:"embedding"`
		}
		type embedResp struct {
			Data []embedData `json:"data"`
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embedResp{Data: []embedData{{Embedding: embedding}}})
	}))
}

func TestEmbed(t *testing.T) {
	want := []float32{0.1, 0.2, 0.3}
	srv := newEmbedServer(t, want)
	defer srv.Close()

	c, _ := New(srv.URL, testEmbedModel, "", "")
	got, err := c.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got len %d, want %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("got[%d] = %v, want %v", i, got[i], v)
		}
	}
}

// mockThinkingWriter records answer and thinking separately.
type mockThinkingWriter struct {
	answer   strings.Builder
	thinking strings.Builder
}

func (m *mockThinkingWriter) Write(p []byte) (int, error)         { return m.answer.Write(p) }
func (m *mockThinkingWriter) WriteThinking(p []byte) (int, error) { return m.thinking.Write(p) }

// newChatServer creates a mock /v1/chat/completions SSE server.
// It emits a thinking chunk followed by an answer chunk.
func newChatServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}

		// Parse request to check include_reasoning flag.
		var req struct {
			IncludeReasoning *bool `json:"include_reasoning"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)

		emit := func(delta map[string]string) {
			chunk := map[string]any{
				"choices": []map[string]any{
					{"delta": delta},
				},
			}
			data, _ := json.Marshal(chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		// Emit thinking unless include_reasoning=false.
		if req.IncludeReasoning == nil || *req.IncludeReasoning {
			emit(map[string]string{"reasoning": "thinking tokens"})
		}
		emit(map[string]string{"content": "answer tokens"})
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

func TestChat(t *testing.T) {
	tests := []struct {
		name               string
		thinkMode          rag.ThinkMode
		wantThinkingRouted bool
		wantThinkingEmpty  bool
	}{
		{
			name:               "ThinkAuto routes thinking to ThinkingWriter",
			thinkMode:          rag.ThinkAuto,
			wantThinkingRouted: true,
		},
		{
			name:               "ThinkHidden suppresses thinking",
			thinkMode:          rag.ThinkHidden,
			wantThinkingRouted: false,
		},
		{
			name:               "ThinkDisabled suppresses thinking and sends flag",
			thinkMode:          rag.ThinkDisabled,
			wantThinkingRouted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newChatServer(t)
			defer srv.Close()

			c, _ := New(srv.URL, testEmbedModel, testChatModel, "")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			mw := &mockThinkingWriter{}
			err := c.Chat(ctx, "system", "", "question", rag.ChatOptions{ThinkMode: tt.thinkMode}, mw)
			if err != nil {
				t.Fatalf("Chat() error = %v", err)
			}

			if tt.wantThinkingRouted && mw.thinking.Len() == 0 {
				t.Error("expected thinking to be routed to WriteThinking, got empty")
			}
			if !tt.wantThinkingRouted && mw.thinking.Len() != 0 {
				t.Errorf("expected thinking suppressed, got: %q", mw.thinking.String())
			}
			if !strings.Contains(mw.answer.String(), "answer tokens") {
				t.Errorf("expected answer in output, got: %q", mw.answer.String())
			}
		})
	}
}

func TestChat_PlainWriter(t *testing.T) {
	srv := newChatServer(t)
	defer srv.Close()

	c, _ := New(srv.URL, testEmbedModel, testChatModel, "")
	var buf strings.Builder
	err := c.Chat(context.Background(), "", "", "q", rag.ChatOptions{ThinkMode: rag.ThinkAuto}, &buf)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if !strings.Contains(buf.String(), "answer tokens") {
		t.Errorf("expected answer, got: %q", buf.String())
	}
}

func TestChat_APIKey(t *testing.T) {
	const wantKey = "secret-token"
	var gotKey string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	c, _ := New(srv.URL, testEmbedModel, testChatModel, wantKey)
	_ = c.Chat(context.Background(), "", "", "q", rag.ChatOptions{}, &strings.Builder{})

	if gotKey != wantKey {
		t.Errorf("got Authorization Bearer %q, want %q", gotKey, wantKey)
	}
}
