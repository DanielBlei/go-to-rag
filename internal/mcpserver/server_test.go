package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DanielBlei/go-to-rag/internal/rag"
	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

// fakeRetriever implements rag.Pipeline for testing.
type fakeRetriever struct {
	context string               // returned by Retrieve (ask_to_rag_system path)
	chunks  []vectorstore.Result // returned by RetrieveChunks (check_rag_knowledge_base path)
	err     error
}

func (f *fakeRetriever) Retrieve(_ context.Context, _ string, _ int) (string, error) {
	return f.context, f.err
}

func (f *fakeRetriever) RetrieveChunks(
	_ context.Context,
	_ string,
	_ int,
) ([]vectorstore.Result, error) {
	return f.chunks, f.err
}

// fakeChatServer implements rag.ChatServer for testing.
type fakeChatServer struct {
	answer   string
	thinking string
	err      error
	gotOpts  rag.ChatOptions
}

func (f *fakeChatServer) Chat(
	_ context.Context,
	_, _, _ string,
	opts rag.ChatOptions,
	w io.Writer,
) error {
	f.gotOpts = opts
	if f.err != nil {
		return f.err
	}
	if f.thinking != "" {
		type thinkingWriter interface {
			WriteThinking([]byte) (int, error)
		}
		if tw, ok := w.(thinkingWriter); ok {
			_, _ = tw.WriteThinking([]byte(f.thinking))
		}
	}
	_, _ = fmt.Fprint(w, f.answer)
	return nil
}

// connectClient connects an in-memory client to the server's MCP instance and
// returns the client session.
func (s *Server) connectClient(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := s.mcpServer.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v1"}, nil)
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestCheckRAGKnowledgeBase(t *testing.T) {
	// parseEnvelope deserialises the MCP text content into a ragEnvelope for
	// structural assertion. It fails the test if the content is not valid JSON.
	parseEnvelope := func(t *testing.T, content []mcp.Content) ragEnvelope {
		t.Helper()
		text := content[0].(*mcp.TextContent).Text
		var env ragEnvelope
		if err := json.Unmarshal([]byte(text), &env); err != nil {
			t.Fatalf("response is not valid JSON: %v\ntext: %s", err, text)
		}
		return env
	}

	tests := []struct {
		name      string
		retriever *fakeRetriever
		check     func(t *testing.T, env ragEnvelope)
		wantErr   bool
	}{
		{
			name: "envelope always contains data notice",
			retriever: &fakeRetriever{chunks: []vectorstore.Result{
				{Text: "chunk one", Source: "doc.txt", Score: 0.9, ChunkIndex: 0},
			}},
			check: func(t *testing.T, env ragEnvelope) {
				if env.DataNotice == "" {
					t.Error("DataNotice is empty")
				}
				if len(env.Chunks) != 1 {
					t.Fatalf("want 1 chunk, got %d", len(env.Chunks))
				}
				if env.Chunks[0].Text != "chunk one" {
					t.Errorf("Chunks[0].Text = %q, want %q", env.Chunks[0].Text, "chunk one")
				}
				if env.Chunks[0].Source != "doc.txt" {
					t.Errorf("Chunks[0].Source = %q, want %q", env.Chunks[0].Source, "doc.txt")
				}
			},
		},
		{
			name: "high confidence chunk does not set low_confidence",
			retriever: &fakeRetriever{chunks: []vectorstore.Result{
				{Text: "reliable chunk", Source: "doc.txt", Score: 0.9, ChunkIndex: 0},
			}},
			check: func(t *testing.T, env ragEnvelope) {
				if len(env.Chunks) != 1 {
					t.Fatalf("want 1 chunk, got %d", len(env.Chunks))
				}
				if env.Chunks[0].LowConfidence {
					t.Error("LowConfidence = true for score 0.9, want false")
				}
			},
		},
		{
			name: "low confidence chunk sets low_confidence flag",
			retriever: &fakeRetriever{chunks: []vectorstore.Result{
				{Text: "weak match", Source: "doc.txt", Score: 0.3, ChunkIndex: 0},
			}},
			check: func(t *testing.T, env ragEnvelope) {
				if len(env.Chunks) != 1 {
					t.Fatalf("want 1 chunk, got %d", len(env.Chunks))
				}
				if !env.Chunks[0].LowConfidence {
					t.Error("LowConfidence = false for score 0.3, want true")
				}
			},
		},
		{
			name:      "empty results returns envelope with empty chunks array",
			retriever: &fakeRetriever{chunks: nil},
			check: func(t *testing.T, env ragEnvelope) {
				if env.DataNotice == "" {
					t.Error("DataNotice is empty on empty-result envelope")
				}
				if env.Chunks == nil {
					t.Error("Chunks is nil, want empty slice")
				}
				if len(env.Chunks) != 0 {
					t.Errorf("want 0 chunks, got %d", len(env.Chunks))
				}
			},
		},
		{
			name:      "retriever error returns tool error",
			retriever: &fakeRetriever{err: errors.New("retriever down")},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := New(tt.retriever, nil, 5, rag.ThinkHidden, 0.5).connectClient(t)
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "check_rag_knowledge_base",
				Arguments: map[string]any{"question": "what are pods?"},
			})
			if err != nil {
				t.Fatalf("unexpected protocol error: %v", err)
			}
			if tt.wantErr {
				if !res.IsError {
					t.Fatal("expected IsError=true, got false")
				}
				return
			}
			if res.IsError {
				t.Fatalf("unexpected tool error: %v", res.Content)
			}
			if len(res.Content) == 0 {
				t.Fatal("expected content, got none")
			}
			env := parseEnvelope(t, res.Content)
			if tt.check != nil {
				tt.check(t, env)
			}
		})
	}
}

func TestAskToRAGSystem(t *testing.T) {
	tests := []struct {
		name       string
		retriever  *fakeRetriever
		chatServer *fakeChatServer
		thinking   string
		think      string
		question   string
		wantErr    bool
		checkOpts  func(t *testing.T, opts rag.ChatOptions)
		checkBody  func(t *testing.T, content []mcp.Content)
	}{
		{
			name:       "happy path with default ThinkHidden",
			retriever:  &fakeRetriever{context: "some context"},
			chatServer: &fakeChatServer{answer: "answer text"},
			question:   "what is this?",
			think:      "",
			checkOpts: func(t *testing.T, opts rag.ChatOptions) {
				if opts.ThinkMode != rag.ThinkHidden {
					t.Errorf("expected ThinkHidden, got %v", opts.ThinkMode)
				}
			},
			checkBody: func(t *testing.T, content []mcp.Content) {
				if len(content) != 1 {
					t.Errorf("expected 1 content item, got %d", len(content))
					return
				}
				text := content[0].(*mcp.TextContent).Text
				if text != "answer text" {
					t.Errorf("expected %q, got %q", "answer text", text)
				}
			},
		},
		{
			name:       "think=disabled disables reasoning",
			retriever:  &fakeRetriever{context: "some context"},
			chatServer: &fakeChatServer{answer: "answer"},
			question:   "what is this?",
			think:      "disabled",
			checkOpts: func(t *testing.T, opts rag.ChatOptions) {
				if opts.ThinkMode != rag.ThinkDisabled {
					t.Errorf("expected ThinkDisabled, got %v", opts.ThinkMode)
				}
			},
			checkBody: func(t *testing.T, content []mcp.Content) {
				if len(content) != 1 {
					t.Errorf("expected 1 content item, got %d", len(content))
				}
			},
		},
		{
			name:       "think=auto with thinking emitted surfaces tokens",
			retriever:  &fakeRetriever{context: "context"},
			chatServer: &fakeChatServer{answer: "answer", thinking: "reasoning steps"},
			question:   "what is this?",
			think:      "auto",
			checkOpts: func(t *testing.T, opts rag.ChatOptions) {
				if opts.ThinkMode != rag.ThinkAuto {
					t.Errorf("expected ThinkAuto, got %v", opts.ThinkMode)
				}
			},
			checkBody: func(t *testing.T, content []mcp.Content) {
				if len(content) != 2 {
					t.Errorf("expected 2 content items, got %d", len(content))
					return
				}
				answerText := content[0].(*mcp.TextContent).Text
				if answerText != "answer" {
					t.Errorf("expected answer %q, got %q", "answer", answerText)
				}
				thinkingText := content[1].(*mcp.TextContent).Text
				if !strings.Contains(thinkingText, "[thinking]") {
					t.Errorf("expected thinking prefix, got %q", thinkingText)
				}
			},
		},
		{
			name:       "empty question returns error",
			retriever:  &fakeRetriever{context: "context"},
			chatServer: &fakeChatServer{answer: "answer"},
			question:   "",
			wantErr:    true,
		},
		{
			name:       "chat server error",
			retriever:  &fakeRetriever{context: "context"},
			chatServer: &fakeChatServer{err: errors.New("chat failed")},
			question:   "what is this?",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := New(tt.retriever, tt.chatServer, 5, rag.ThinkHidden, 0.5).connectClient(t)
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "ask_to_rag_system",
				Arguments: map[string]any{
					"question": tt.question,
					"think":    tt.think,
				},
			})
			if err != nil {
				t.Fatalf("unexpected protocol error: %v", err)
			}
			if tt.wantErr {
				if !res.IsError {
					t.Fatal("expected IsError=true, got false")
				}
				return
			}
			if res.IsError {
				t.Fatalf("unexpected tool error: %v", res.Content)
			}
			if len(res.Content) == 0 {
				t.Fatal("expected content, got none")
			}

			if tt.checkOpts != nil {
				tt.checkOpts(t, tt.chatServer.gotOpts)
			}
			if tt.checkBody != nil {
				tt.checkBody(t, res.Content)
			}
		})
	}
}
