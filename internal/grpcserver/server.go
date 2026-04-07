package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	ragv1 "github.com/DanielBlei/go-to-rag/internal/gen/rag/v1"
	"github.com/DanielBlei/go-to-rag/internal/rag"
)

// protoThinkMode maps from proto enum to rag.ThinkMode.
func protoThinkMode(m ragv1.ThinkMode) rag.ThinkMode {
	switch m {
	case ragv1.ThinkMode_THINK_MODE_DISABLED:
		return rag.ThinkDisabled
	case ragv1.ThinkMode_THINK_MODE_HIDDEN:
		return rag.ThinkHidden
	default:
		return rag.ThinkAuto
	}
}

// Server wraps a rag.Pipeline and serves it over gRPC.
type Server struct {
	ragv1.UnimplementedRAGServiceServer
	retriever         rag.Pipeline
	chatServer        rag.ChatServer
	topK              int
	serveWithFallback bool
	defaultThinkMode  rag.ThinkMode
	srv               *grpc.Server
}

func unaryLoggingInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	ev := log.Info()
	if err != nil {
		ev = log.Error().Err(err)
	}
	ev.Str("method", info.FullMethod).
		Str("code", status.Code(err).String()).
		Int64("duration_ms", time.Since(start).Milliseconds()).
		Msg("rpc")
	return resp, err
}

func streamLoggingInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()
	err := handler(srv, ss)
	ev := log.Info()
	if err != nil {
		ev = log.Error().Err(err)
	}
	ev.Str("method", info.FullMethod).
		Str("code", status.Code(err).String()).
		Int64("duration_ms", time.Since(start).Milliseconds()).
		Msg("rpc")
	return err
}

// New creates a gRPC server backed by the given Pipeline and ChatServer.
// opts are forwarded to grpc.NewServer and can be used to configure TLS,
// interceptors, and other server-level options.
func New(retriever rag.Pipeline, chatServer rag.ChatServer, topK int, serveWithFallback bool, defaultThinkMode rag.ThinkMode, opts ...grpc.ServerOption) *Server {
	s := &Server{
		retriever:         retriever,
		chatServer:        chatServer,
		topK:              topK,
		serveWithFallback: serveWithFallback,
		defaultThinkMode:  defaultThinkMode,
	}
	serverOpts := append([]grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unaryLoggingInterceptor),
		grpc.ChainStreamInterceptor(streamLoggingInterceptor),
	}, opts...)
	s.srv = grpc.NewServer(serverOpts...)
	ragv1.RegisterRAGServiceServer(s.srv, s)
	reflection.Register(s.srv)

	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s.srv, healthSrv)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus("rag.v1.RAGService", grpc_health_v1.HealthCheckResponse_SERVING)

	return s
}

// validateQuestion check if questions is valid
// placeholder for potential future checks
func validateQuestion(question string) (string, error) {
	if question == "" {
		return "", status.Error(codes.InvalidArgument, "question is required")
	}
	return question, nil
}

// streamWriter bridges rag.Ask's io.Writer interface to a gRPC server stream,
// forwarding each token chunk as an AskResponse message.
type streamWriter struct {
	stream ragv1.RAGService_AskServer
}

func (sw *streamWriter) Write(p []byte) (int, error) {
	err := sw.stream.Send(&ragv1.AskResponse{
		Content: &ragv1.AskResponse_Answer{Answer: string(p)},
	})
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// WriteThinking implements rag.ThinkingWriter.
func (sw *streamWriter) WriteThinking(p []byte) (int, error) {
	err := sw.stream.Send(&ragv1.AskResponse{
		Content: &ragv1.AskResponse_Thinking{Thinking: string(p)},
	})
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// Ask queries the knowledge base and streams the LLM-generated answer token by token.
// Clients may consume tokens incrementally or drain the stream and concatenate all Answer fields for full response.
func (s *Server) Ask(req *ragv1.AskRequest, stream ragv1.RAGService_AskServer) error {
	question, err := validateQuestion(req.Question)
	if err != nil {
		return err
	}

	log.Debug().Str("question", question).Msg("ask")

	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = s.topK
	}

	// Resolve think_mode: use request value if set, else server default.
	thinkMode := s.defaultThinkMode
	if req.ThinkMode != nil {
		thinkMode = protoThinkMode(req.GetThinkMode())
	}
	chatOpts := rag.ChatOptions{ThinkMode: thinkMode}

	ctx := stream.Context()
	_, askErr := rag.Ask(ctx, s.retriever, s.chatServer, question, topK, s.serveWithFallback, chatOpts, &streamWriter{stream: stream})
	if askErr != nil {
		if errors.Is(askErr, context.DeadlineExceeded) {
			return status.Error(codes.DeadlineExceeded, "request timed out")
		}
		if errors.Is(askErr, context.Canceled) {
			return status.Error(codes.Canceled, "request cancelled")
		}
		return status.Errorf(codes.Internal, "ask: %v", askErr)
	}
	return nil
}

// GetServerConfig returns the server's current default configurations.
func (s *Server) GetServerConfig(_ context.Context, _ *ragv1.GetServerConfigRequest) (*ragv1.ServerConfig, error) {
	return &ragv1.ServerConfig{
		DefaultThinkMode: ragv1.ThinkMode(s.defaultThinkMode),
	}, nil
}

// RetrieveChunks returns scored chunks from the vector store.
func (s *Server) RetrieveChunks(ctx context.Context, req *ragv1.RetrieveChunksRequest) (*ragv1.RetrieveChunksResponse, error) {
	question, err := validateQuestion(req.Question)
	if err != nil {
		return nil, err
	}

	log.Debug().Str("question", question).Msg("retrieve_chunks")

	// option to overwrite the topK matches from server defaults
	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = s.topK
	}

	chunks, err := s.retriever.RetrieveChunks(ctx, question, topK)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "request timed out")
		}
		if errors.Is(err, context.Canceled) {
			return nil, status.Error(codes.Canceled, "request cancelled")
		}
		return nil, status.Errorf(codes.Internal, "retrieve chunks: %v", err)
	}

	resp := &ragv1.RetrieveChunksResponse{
		Chunks: make([]*ragv1.Chunk, len(chunks)),
	}
	for i, c := range chunks {
		resp.Chunks[i] = &ragv1.Chunk{
			Text:       c.Text,
			Source:     c.Source,
			Score:      c.Score,
			ChunkIndex: int32(c.ChunkIndex),
		}
	}
	return resp, nil
}

// ServeListener starts the gRPC server on the given listener and blocks until the context is cancelled.
// Use this when the caller pre-binds the port (e.g. in tests to avoid listen-close-rebind races).
func (s *Server) ServeListener(ctx context.Context, lis net.Listener) error {
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			log.Info().Msg("gRPC server: graceful shutdown")
			s.srv.GracefulStop()
		case <-done:
		}
	}()

	log.Info().Str("addr", lis.Addr().String()).Msg("gRPC server listening")
	if err := s.srv.Serve(lis); err != nil {
		return fmt.Errorf("grpc serve: %w", err)
	}
	return nil
}

// Serve starts the gRPC server on the given address and blocks until the context is cancelled.
func (s *Server) Serve(ctx context.Context, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	return s.ServeListener(ctx, lis)
}
