package inference

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	ollamaEmbedModel = "mxbai-embed-large:latest"
	ollamaChatModel  = "llama3.2:1b"
	vllmEmbedModel   = "BAAI/bge-large-en-v1.5"
	vllmChatModel    = "meta-llama/Llama-3.1-8B-Instruct"
)

// newOllamaTagsServer serves GET /api/tags with the given model names.
func newOllamaTagsServer(t *testing.T, models []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		type m struct {
			Model string `json:"model"`
		}
		type resp struct {
			Models []m `json:"models"`
		}
		body := resp{}
		for _, name := range models {
			body.Models = append(body.Models, m{Model: name})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

// newVLLMModelsServer serves GET /v1/models with the given model IDs.
func newVLLMModelsServer(t *testing.T, ids []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		type d struct {
			ID string `json:"id"`
		}
		type resp struct {
			Data []d `json:"data"`
		}
		body := resp{}
		for _, id := range ids {
			body.Data = append(body.Data, d{ID: id})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func ctx2s(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestResolve_UnknownProvider(t *testing.T) {
	_, _, err := Resolve(context.Background(), "foobar", "http://localhost", "", "", "", false, false)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unknown inference provider") {
		t.Errorf("error should mention 'unknown inference provider', got: %v", err)
	}
}

func TestResolve_vLLM_BothModels(t *testing.T) {
	srv := newVLLMModelsServer(t, []string{vllmEmbedModel, vllmChatModel})
	defer srv.Close()

	embedder, chat, err := Resolve(ctx2s(t), "vllm", srv.URL, vllmEmbedModel, vllmChatModel, "", true, true)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if embedder == nil {
		t.Error("expected non-nil embedder")
	}
	if chat == nil {
		t.Error("expected non-nil chatServer")
	}
}

func TestResolve_vLLM_EmbedOnly(t *testing.T) {
	srv := newVLLMModelsServer(t, []string{vllmEmbedModel})
	defer srv.Close()

	embedder, chat, err := Resolve(ctx2s(t), "vllm", srv.URL, vllmEmbedModel, "", "", true, false)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if embedder == nil {
		t.Error("expected non-nil embedder")
	}
	if chat != nil {
		t.Errorf("expected nil chatServer when chatModel is empty, got %v", chat)
	}
}

func TestResolve_vLLM_Unreachable(t *testing.T) {
	_, _, err := Resolve(ctx2s(t), "vllm", "http://127.0.0.1:1", vllmEmbedModel, vllmChatModel, "", true, true)
	if err == nil {
		t.Fatal("expected error for unreachable host")
	}
	if !strings.Contains(err.Error(), "backend validation") {
		t.Errorf("error should mention 'backend validation', got: %v", err)
	}
}

func TestResolve_Ollama_BothModels(t *testing.T) {
	srv := newOllamaTagsServer(t, []string{ollamaEmbedModel, ollamaChatModel})
	defer srv.Close()

	embedder, chat, err := Resolve(ctx2s(t), "ollama", srv.URL, ollamaEmbedModel, ollamaChatModel, "", true, true)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if embedder == nil {
		t.Error("expected non-nil embedder")
	}
	if chat == nil {
		t.Error("expected non-nil chatServer")
	}
}

func TestResolve_Ollama_EmbedOnly(t *testing.T) {
	srv := newOllamaTagsServer(t, []string{ollamaEmbedModel})
	defer srv.Close()

	embedder, chat, err := Resolve(ctx2s(t), "ollama", srv.URL, ollamaEmbedModel, "", "", true, false)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if embedder == nil {
		t.Error("expected non-nil embedder")
	}
	if chat != nil {
		t.Errorf("expected nil chatServer when chatModel is empty, got %v", chat)
	}
}
