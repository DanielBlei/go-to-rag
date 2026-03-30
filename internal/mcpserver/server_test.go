package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

// fakeRetriever implements rag.Pipeline for testing.
type fakeRetriever struct {
	context string
	err     error
}

func (f *fakeRetriever) Retrieve(_ context.Context, _ string, _ int) (string, error) {
	return f.context, f.err
}

func (f *fakeRetriever) RetrieveChunks(_ context.Context, _ string, _ int) ([]vectorstore.Result, error) {
	return nil, f.err
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

func TestAskToRAGSystem(t *testing.T) {
	tests := []struct {
		name      string
		retriever *fakeRetriever
		wantText  string
		wantErr   bool
	}{
		{
			name:      "returns framed context",
			retriever: &fakeRetriever{context: "chunk one\n---\nchunk two"},
			wantText:  "Use the following knowledge base context",
		},
		{
			name:      "empty results returns no documents message",
			retriever: &fakeRetriever{context: ""},
			wantText:  "no relevant documents found",
		},
		{
			name:      "retriever error returns tool error",
			retriever: &fakeRetriever{err: errors.New("retriever down")},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := New(tt.retriever, 5).connectClient(t)
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "ask_to_rag_system",
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
			text := res.Content[0].(*mcp.TextContent).Text
			if !strings.Contains(text, tt.wantText) {
				t.Errorf("text %q does not contain %q", text, tt.wantText)
			}
		})
	}
}
