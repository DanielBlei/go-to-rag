package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ragv1 "github.com/DanielBlei/go-to-rag/internal/gen/rag/v1"
	"github.com/DanielBlei/go-to-rag/internal/rag"
)

// Server wraps a rag.Pipeline and serves it over gRPC.
type Server struct {
	ragv1.UnimplementedRAGServiceServer
	retriever    rag.Pipeline
	chatServer   rag.ChatServer
	topK         int
	withFallback bool
	srv          *grpc.Server
}

// New creates a gRPC server backed by the given Pipeline and ChatServer.
// opts are forwarded to grpc.NewServer and can be used to configure TLS,
// interceptors, and other server-level options.
func New(retriever rag.Pipeline, chatServer rag.ChatServer, topK int, withFallback bool, opts ...grpc.ServerOption) *Server {
	s := &Server{
		retriever:    retriever,
		chatServer:   chatServer,
		topK:         topK,
		withFallback: withFallback,
	}
	s.srv = grpc.NewServer(opts...)
	ragv1.RegisterRAGServiceServer(s.srv, s)
	return s
}

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
	if err := sw.stream.Send(&ragv1.AskResponse{Answer: string(p)}); err != nil {
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

	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = s.topK
	}

	ctx := stream.Context()
	if _, err := rag.Ask(ctx, s.retriever, s.chatServer, question, topK, s.withFallback, &streamWriter{stream}); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return status.Error(codes.Canceled, "request cancelled")
		}
		return status.Errorf(codes.Internal, "ask: %v", err)
	}
	return nil
}

// RetrieveChunks returns scored chunks from the vector store.
func (s *Server) RetrieveChunks(ctx context.Context, req *ragv1.RetrieveChunksRequest) (*ragv1.RetrieveChunksResponse, error) {
	question, err := validateQuestion(req.Question)
	if err != nil {
		return nil, err
	}

	// option to overwrite the topK matches from server defaults
	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = s.topK
	}

	chunks, err := s.retriever.RetrieveChunks(ctx, question, topK)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
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

// Serve starts the gRPC server on the given address and blocks until
// the context is cancelled.
func (s *Server) Serve(ctx context.Context, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	go func() {
		<-ctx.Done()
		log.Info().Msg("gRPC server: graceful shutdown")
		s.srv.GracefulStop()
	}()

	log.Info().Str("addr", addr).Msg("gRPC server listening")
	if err := s.srv.Serve(lis); err != nil {
		return fmt.Errorf("grpc serve: %w", err)
	}
	return nil
}
