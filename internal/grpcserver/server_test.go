package grpcserver

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	ragv1 "github.com/DanielBlei/go-to-rag/internal/gen/rag/v1"
	"github.com/DanielBlei/go-to-rag/internal/rag"
	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

// thinkingWriter mirrors ollama.ThinkingWriter for test-local type assertions.
type thinkingWriter interface {
	WriteThinking(p []byte) (int, error)
}

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

// slowFakeRetriever simulates a slow operation that respects context cancellation.
type slowFakeRetriever struct {
	chunks []vectorstore.Result
	delay  time.Duration
}

func (f *slowFakeRetriever) Retrieve(_ context.Context, _ string, _ int) (string, error) {
	return "slow", nil
}

func (f *slowFakeRetriever) RetrieveChunks(ctx context.Context, _ string, _ int) ([]vectorstore.Result, error) {
	select {
	case <-time.After(f.delay):
		return f.chunks, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type fakeChatServer struct {
	response string
	thinking string // if non-empty, routed via thinkingWriter if w implements it
	err      error
}

func (f *fakeChatServer) Chat(_ context.Context, _, _, _ string, opts rag.ChatOptions, w io.Writer) error {
	if f.err != nil {
		return f.err
	}
	// Suppress thinking for ThinkHidden or ThinkDisabled modes, matching ollama.Client behavior.
	if f.thinking != "" && opts.ThinkMode != rag.ThinkHidden && opts.ThinkMode != rag.ThinkDisabled {
		if tw, ok := w.(thinkingWriter); ok {
			_, _ = tw.WriteThinking([]byte(f.thinking))
		}
	}
	_, _ = w.Write([]byte(f.response))
	return nil
}

// slowFakeChatServer delays before writing to simulate a slow operation that respects cancellation.
type slowFakeChatServer struct {
	response string
	err      error
	delay    time.Duration
}

func (f *slowFakeChatServer) Chat(ctx context.Context, _, _, _ string, _ rag.ChatOptions, w io.Writer) error {
	if f.err != nil {
		return f.err
	}
	// Simulate a slow operation that respects context cancellation
	select {
	case <-time.After(f.delay):
		_, _ = w.Write([]byte(f.response))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func dialBufconn(t *testing.T, s *Server) ragv1.RAGServiceClient {
	t.Helper()
	conn := dialBufconnConn(t, s)
	return ragv1.NewRAGServiceClient(conn)
}

func dialBufconnConn(t *testing.T, s *Server) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = lis.Close() })

	go func() {
		if err := s.srv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
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
	return conn
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
			srv := New(tt.retriever, &fakeChatServer{}, 10, false, rag.ThinkAuto)
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
	srv := New(&fakeRetriever{chunks: []vectorstore.Result{want}}, &fakeChatServer{}, 10, false, rag.ThinkAuto)
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

// drainAsk opens an Ask stream and collects all answer and thinking chunks into separate strings.
// Errors from stream establishment or Recv are returned directly.
func drainAsk(t *testing.T, ctx context.Context, client ragv1.RAGServiceClient, req *ragv1.AskRequest) (answer, thinking string, err error) {
	t.Helper()
	stream, err := client.Ask(ctx, req)
	if err != nil {
		return "", "", err
	}
	var answerSb, thinkingSb strings.Builder
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", "", err
		}
		answerSb.WriteString(msg.GetAnswer())
		thinkingSb.WriteString(msg.GetThinking())
	}
	return answerSb.String(), thinkingSb.String(), nil
}

func TestAsk(t *testing.T) {
	tests := []struct {
		name         string
		retriever    *fakeRetriever
		chatServer   *fakeChatServer
		thinkMode    *ragv1.ThinkMode // nil = use server default
		wantAnswer   string
		wantThinking string
		wantErr      bool
	}{
		{
			name: "streams and assembles generated answer",
			retriever: &fakeRetriever{chunks: []vectorstore.Result{
				{Text: "pods are the smallest deployable unit"},
			}},
			chatServer: &fakeChatServer{response: "Pods are the smallest deployable unit in Kubernetes."},
			thinkMode:  nil, // use server default
			wantAnswer: "Pods are the smallest deployable unit in Kubernetes.",
		},
		{
			name:       "empty context still generates answer",
			retriever:  &fakeRetriever{chunks: nil},
			chatServer: &fakeChatServer{response: "I don't have context for that."},
			thinkMode:  nil,
			wantAnswer: "I don't have context for that.",
		},
		{
			name:       "retriever error propagates",
			retriever:  &fakeRetriever{err: errors.New("embed failed")},
			chatServer: &fakeChatServer{},
			thinkMode:  nil,
			wantErr:    true,
		},
		{
			name: "chat error propagates",
			retriever: &fakeRetriever{chunks: []vectorstore.Result{
				{Text: "some context"},
			}},
			chatServer: &fakeChatServer{err: errors.New("model unavailable")},
			thinkMode:  nil,
			wantErr:    true,
		},
		{
			name:         "thinking forwarded with ThinkAuto",
			retriever:    &fakeRetriever{chunks: nil},
			chatServer:   &fakeChatServer{response: "answer", thinking: "let me think"},
			thinkMode:    ptrThinkMode(ragv1.ThinkMode_THINK_MODE_AUTO),
			wantAnswer:   "answer",
			wantThinking: "let me think",
		},
		{
			name:         "thinking suppressed with ThinkHidden",
			retriever:    &fakeRetriever{chunks: nil},
			chatServer:   &fakeChatServer{response: "answer", thinking: "let me think"},
			thinkMode:    ptrThinkMode(ragv1.ThinkMode_THINK_MODE_HIDDEN),
			wantAnswer:   "answer",
			wantThinking: "",
		},
		{
			name:       "no thinking field works fine",
			retriever:  &fakeRetriever{chunks: nil},
			chatServer: &fakeChatServer{response: "answer"},
			thinkMode:  ptrThinkMode(ragv1.ThinkMode_THINK_MODE_AUTO),
			wantAnswer: "answer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Server default is ThinkAuto
			srv := New(tt.retriever, tt.chatServer, 10, false, rag.ThinkAuto)
			client := dialBufconn(t, srv)

			req := &ragv1.AskRequest{Question: "what are pods?"}
			if tt.thinkMode != nil {
				req.ThinkMode = tt.thinkMode
			}
			answer, thinking, err := drainAsk(t, context.Background(), client, req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if answer != tt.wantAnswer {
				t.Errorf("answer = %q, want %q", answer, tt.wantAnswer)
			}
			if thinking != tt.wantThinking {
				t.Errorf("thinking = %q, want %q", thinking, tt.wantThinking)
			}
		})
	}
}

// ptrThinkMode is a helper to create a pointer to a ThinkMode enum value.
func ptrThinkMode(mode ragv1.ThinkMode) *ragv1.ThinkMode {
	return &mode
}

// TestContextErrors verifies that both Ask and RetrieveChunks correctly map context errors
// (cancellation and deadline exceeded) to appropriate gRPC status codes.
// The handlers use errors.Is on the operation error, which walks the chain to find
// context.DeadlineExceeded or context.Canceled regardless of wrapping depth.
func TestContextErrors(t *testing.T) {
	tests := []struct {
		name       string
		isAsk      bool // true for Ask, false for RetrieveChunks
		ctxFn      func() context.Context
		wantCode   codes.Code
		wantErrMsg string
	}{
		{
			name:  "Ask context cancelled",
			isAsk: true,
			ctxFn: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				go func() {
					time.Sleep(5 * time.Millisecond)
					cancel()
				}()
				return ctx
			},
			wantCode: codes.Canceled,
		},
		{
			name:  "Ask deadline exceeded",
			isAsk: true,
			ctxFn: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				_ = cancel // Will be cancelled by timeout
				return ctx
			},
			wantCode: codes.DeadlineExceeded,
		},
		{
			name:  "RetrieveChunks context cancelled",
			isAsk: false,
			ctxFn: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				go func() {
					time.Sleep(5 * time.Millisecond)
					cancel()
				}()
				return ctx
			},
			wantCode: codes.Canceled,
		},
		{
			name:  "RetrieveChunks deadline exceeded",
			isAsk: false,
			ctxFn: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				_ = cancel // Will be cancelled by timeout
				return ctx
			},
			wantCode: codes.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up slow operations to exceed deadline/cancellation
			retriever := &slowFakeRetriever{
				chunks: []vectorstore.Result{{Text: "ctx"}},
				delay:  500 * time.Millisecond,
			}
			chatServer := &slowFakeChatServer{
				response: "slow answer",
				delay:    500 * time.Millisecond,
			}
			srv := New(retriever, chatServer, 10, false, rag.ThinkAuto)
			client := dialBufconn(t, srv)

			ctx := tt.ctxFn()

			if tt.isAsk {
				// Ask RPC test
				stream, err := client.Ask(ctx, &ragv1.AskRequest{Question: "test?"})
				if err != nil {
					// Stream establishment itself may fail with DeadlineExceeded — acceptable.
					st, ok := status.FromError(err)
					if !ok || st.Code() != tt.wantCode {
						t.Errorf("stream creation error code = %v, want %v", st.Code(), tt.wantCode)
					}
					return
				}
				_, err = stream.Recv()
				if err == nil {
					t.Fatal("expected error due to context issue, got nil")
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Fatalf("error is not a gRPC status: %v", err)
				}
				if st.Code() != tt.wantCode {
					t.Errorf("got status code %v, want %v", st.Code(), tt.wantCode)
				}
			} else {
				// RetrieveChunks RPC test
				_, err := client.RetrieveChunks(ctx, &ragv1.RetrieveChunksRequest{Question: "test?", TopK: 1})
				if err == nil {
					t.Fatal("expected error due to context issue, got nil")
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Fatalf("error is not a gRPC status: %v", err)
				}
				if st.Code() != tt.wantCode {
					t.Errorf("got status code %v, want %v", st.Code(), tt.wantCode)
				}
			}
		})
	}
}

func TestGetServerConfig(t *testing.T) {
	tests := []struct {
		name                 string
		defaultThinkMode     rag.ThinkMode
		wantDefaultThinkMode ragv1.ThinkMode
	}{
		{
			name:                 "ThinkAuto default",
			defaultThinkMode:     rag.ThinkAuto,
			wantDefaultThinkMode: ragv1.ThinkMode_THINK_MODE_AUTO,
		},
		{
			name:                 "ThinkDisabled default",
			defaultThinkMode:     rag.ThinkDisabled,
			wantDefaultThinkMode: ragv1.ThinkMode_THINK_MODE_DISABLED,
		},
		{
			name:                 "ThinkHidden default",
			defaultThinkMode:     rag.ThinkHidden,
			wantDefaultThinkMode: ragv1.ThinkMode_THINK_MODE_HIDDEN,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(&fakeRetriever{}, &fakeChatServer{}, 10, false, tt.defaultThinkMode)
			client := dialBufconn(t, srv)

			resp, err := client.GetServerConfig(context.Background(), &ragv1.GetServerConfigRequest{})
			if err != nil {
				t.Fatalf("GetServerConfig: %v", err)
			}
			if resp.DefaultThinkMode != tt.wantDefaultThinkMode {
				t.Errorf("DefaultThinkMode = %v, want %v", resp.DefaultThinkMode, tt.wantDefaultThinkMode)
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	srv := New(&fakeRetriever{chunks: nil}, &fakeChatServer{}, 10, false, rag.ThinkAuto)
	conn := dialBufconnConn(t, srv)

	client := grpc_health_v1.NewHealthClient(conn)

	for _, service := range []string{"", "rag.v1.RAGService"} {
		resp, err := client.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{Service: service})
		if err != nil {
			t.Fatalf("service=%q: %v", service, err)
		}
		if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("service=%q: got %v, want SERVING", service, resp.Status)
		}
	}
}
