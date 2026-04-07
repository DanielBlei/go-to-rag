package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/rag"
)

func TestRunAsk_ThinkFlag(t *testing.T) {
	// Create a mock Ollama server that handles /api/tags and /api/chat.
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			models := []api.ListModelResponse{
				{Model: testEmbedModel},
				{Model: testChatModel},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(api.ListResponse{Models: models})
		case "/api/chat":
			w.Header().Set("Content-Type", "application/x-ndjson")
			flusher := w.(http.Flusher)
			// Return minimal answer response.
			answerResp := api.ChatResponse{Message: api.Message{Content: "test answer"}}
			_ = json.NewEncoder(w).Encode(answerResp)
			flusher.Flush()
			// Done sentinel.
			doneResp := api.ChatResponse{Done: true}
			_ = json.NewEncoder(w).Encode(doneResp)
			flusher.Flush()
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollamaServer.Close()

	tests := []struct {
		name      string
		thinkMode rag.ThinkMode
		wantErr   bool
	}{
		{
			name:      "auto is valid",
			thinkMode: rag.ThinkAuto,
			wantErr:   false,
		},
		{
			name:      "disabled is valid",
			thinkMode: rag.ThinkDisabled,
			wantErr:   false,
		},
		{
			name:      "hidden is valid",
			thinkMode: rag.ThinkHidden,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set package-level vars used by runAsk.
			host = ollamaServer.URL
			embedModel = testEmbedModel
			chatModel = testChatModel
			thinkMode = tt.thinkMode
			dbPath = t.TempDir() + "/nonexistent.db" // Store doesn't exist; Chat path is taken
			topK = 5
			withFallback = false

			cmd := &cobra.Command{}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cmd.SetContext(ctx)

			// Call runAsk directly with a dummy prompt.
			err := runAsk(cmd, []string{"test question"})

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	// Test invalid value at flag-parse time.
	t.Run("invalid value errors at flag parse", func(t *testing.T) {
		var tm rag.ThinkMode
		flag := &thinkModeFlag{val: &tm}
		err := flag.Set("banana")
		if err == nil {
			t.Fatal("expected error for invalid value, got nil")
		}
	})
}
