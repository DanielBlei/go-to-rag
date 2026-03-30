package grpcserver

import (
	"bytes"
	"context"
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
func New(retriever rag.Pipeline, chatServer rag.ChatServer, topK int, withFallback bool) *Server {
	s := &Server{
		retriever:    retriever,
		chatServer:   chatServer,
		topK:         topK,
		withFallback: withFallback,
	}
	s.srv = grpc.NewServer()
	ragv1.RegisterRAGServiceServer(s.srv, s)
	return s
}

func retrieveQuestion(question string) (string, error) {
	if question == "" {
		return "", status.Error(codes.InvalidArgument, "question is required")
	}
	return question, nil
}

// Ask queries the knowledge base and returns an LLM-generated answer.
func (s *Server) Ask(ctx context.Context, req *ragv1.AskRequest) (*ragv1.AskResponse, error) {
	question, err := retrieveQuestion(req.Question)
	if err != nil {
		return nil, err
	}

	// option to overwrite the topK matches from server defaults
	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = s.topK
	}

	var buf bytes.Buffer
	if _, err := rag.Ask(ctx, s.retriever, s.chatServer, question, topK, s.withFallback, &buf); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	return &ragv1.AskResponse{Answer: buf.String()}, nil
}

// RetrieveChunks returns scored chunks from the vector store.
func (s *Server) RetrieveChunks(ctx context.Context, req *ragv1.RetrieveChunksRequest) (*ragv1.RetrieveChunksResponse, error) {
	question, err := retrieveQuestion(req.Question)
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
