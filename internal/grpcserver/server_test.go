package grpcserver

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	ragv1 "github.com/DanielBlei/go-to-rag/internal/gen/rag/v1"
	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

type fakeRetriever struct {
	text   string
	chunks []vectorstore.Result
	err    error
}

func (f *fakeRetriever) Retrieve(_ context.Context, _ string, _ int) (string, error) {
	return f.text, f.err
}

func (f *fakeRetriever) RetrieveChunks(_ context.Context, _ string, _ int) ([]vectorstore.Result, error) {
	return f.chunks, f.err
}

type fakeChatServer struct {
	response string
	err      error
}

func (f *fakeChatServer) Chat(_ context.Context, _, _, _ string, w io.Writer) error {
	if f.err != nil {
		return f.err
	}
	_, _ = w.Write([]byte(f.response))
	return nil
}

func dialBufconn(t *testing.T, s *Server) ragv1.RAGServiceClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = lis.Close() })

	go func() { _ = s.srv.Serve(lis) }()
	t.Cleanup(func() { s.srv.GracefulStop() })

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return ragv1.NewRAGServiceClient(conn)
}

func TestRetrieveChunks(t *testing.T) {
	tests := []struct {
		name       string
		retriever  *fakeRetriever
		topK       int32
		wantChunks int
		wantErr    bool
	}{
		{
			name: "returns structured chunks",
			retriever: &fakeRetriever{chunks: []vectorstore.Result{
				{Source: "k8s.md", Text: "chunk one", ChunkIndex: 0, Score: 0.95},
				{Source: "k8s.md", Text: "chunk two", ChunkIndex: 1, Score: 0.80},
			}},
			topK:       5,
			wantChunks: 2,
		},
		{
			name:       "empty results returns zero chunks",
			retriever:  &fakeRetriever{chunks: nil},
			topK:       5,
			wantChunks: 0,
		},
		{
			name:      "retriever error returns gRPC error",
			retriever: &fakeRetriever{err: errors.New("embed failed")},
			topK:      5,
			wantErr:   true,
		},
		{
			name: "zero top_k uses server default",
			retriever: &fakeRetriever{chunks: []vectorstore.Result{
				{Text: "chunk"},
			}},
			topK:       0,
			wantChunks: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(tt.retriever, &fakeChatServer{}, 10, false)
			client := dialBufconn(t, srv)

			resp, err := client.RetrieveChunks(context.Background(), &ragv1.RetrieveChunksRequest{
				Question: "what are pods?",
				TopK:     tt.topK,
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resp.GetChunks()) != tt.wantChunks {
				t.Errorf("got %d chunks, want %d", len(resp.GetChunks()), tt.wantChunks)
			}
		})
	}
}

func TestRetrieveChunks_fieldRoundTrip(t *testing.T) {
	want := vectorstore.Result{
		Source: "olm.md", Text: "operator lifecycle", ChunkIndex: 3, Score: 0.92,
	}
	srv := New(&fakeRetriever{chunks: []vectorstore.Result{want}}, &fakeChatServer{}, 10, false)
	client := dialBufconn(t, srv)

	resp, err := client.RetrieveChunks(context.Background(), &ragv1.RetrieveChunksRequest{
		Question: "what is olm?",
		TopK:     1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := resp.GetChunks()[0]
	if got.GetSource() != want.Source {
		t.Errorf("Source = %q, want %q", got.GetSource(), want.Source)
	}
	if got.GetText() != want.Text {
		t.Errorf("Text = %q, want %q", got.GetText(), want.Text)
	}
	if int(got.GetChunkIndex()) != want.ChunkIndex {
		t.Errorf("ChunkIndex = %d, want %d", got.GetChunkIndex(), want.ChunkIndex)
	}
	if got.GetScore() != want.Score {
		t.Errorf("Score = %f, want %f", got.GetScore(), want.Score)
	}
}

func TestAsk(t *testing.T) {
	tests := []struct {
		name       string
		retriever  *fakeRetriever
		chatServer *fakeChatServer
		wantAnswer string
		wantErr    bool
	}{
		{
			name: "returns generated answer",
			retriever: &fakeRetriever{chunks: []vectorstore.Result{
				{Text: "pods are the smallest deployable unit"},
			}},
			chatServer: &fakeChatServer{response: "Pods are the smallest deployable unit in Kubernetes."},
			wantAnswer: "Pods are the smallest deployable unit in Kubernetes.",
		},
		{
			name:       "empty context still generates answer",
			retriever:  &fakeRetriever{chunks: nil},
			chatServer: &fakeChatServer{response: "I don't have context for that."},
			wantAnswer: "I don't have context for that.",
		},
		{
			name:       "retriever error propagates",
			retriever:  &fakeRetriever{err: errors.New("embed failed")},
			chatServer: &fakeChatServer{},
			wantErr:    true,
		},
		{
			name: "chat error propagates",
			retriever: &fakeRetriever{chunks: []vectorstore.Result{
				{Text: "some context"},
			}},
			chatServer: &fakeChatServer{err: errors.New("model unavailable")},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(tt.retriever, tt.chatServer, 10, false)
			client := dialBufconn(t, srv)

			resp, err := client.Ask(context.Background(), &ragv1.AskRequest{
				Question: "what are pods?",
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.GetAnswer() != tt.wantAnswer {
				t.Errorf("answer = %q, want %q", resp.GetAnswer(), tt.wantAnswer)
			}
		})
	}
}
