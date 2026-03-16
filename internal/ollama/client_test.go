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
			embedModel: "nomic-embed-text",
			chatModel:  "llama3.2:1b",
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
		{Model: "llama3.2:1b"},
		{Model: "nomic-embed-text:latest"},
	}

	tests := []struct {
		want      string
		available bool
	}{
		{"llama3.2:1b", true},
		{"nomic-embed-text", true}, // should match ":latest" suffix
		{"nomic-embed-text:latest", true},
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

			c, err := New(host, "nomic-embed-text", "llama3.2:1b")
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
