package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"

	"github.com/DanielBlei/go-to-rag/internal/rag"
)

const askToRAGSystemDescription = `
	Search the local knowledge base and return relevant context for a given question.
	Use this tool whenever the user asks about a topic that may be covered in the knowledge base.
	Pass the user's question as-is.
	The tool returns text context separated by "---" that you should use as primary context when answering.
	If the response says "no relevant documents found", answer from your own knowledge and you HAVE TO say so.`

// Server wraps an MCP server with retrieval capabilities.
type Server struct {
	retriever rag.Pipeline
	mcpServer *mcp.Server
	topK      int
}

// New creates an MCP server backed by the given Pipeline.
func New(retriever rag.Pipeline, topK int) *Server {
	s := &Server{retriever: retriever, topK: topK}
	s.mcpServer = mcp.NewServer(&mcp.Implementation{Name: "go-to-rag", Version: "0.1.0"}, nil)
	s.registerTools()
	return s
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "ask_to_rag_system",
		Description: askToRAGSystemDescription,
	}, s.askToRAGSystem)
}

// ServeStdio connects the MCP server to stdin/stdout and blocks until the
// connection closes or ctx is cancelled.
func (s *Server) ServeStdio(ctx context.Context) error {
	sess, err := s.mcpServer.Connect(ctx, &mcp.StdioTransport{}, nil)
	if err != nil {
		return fmt.Errorf("mcp connect: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = sess.Close()
	}()
	return sess.Wait()
}

// ServeSSE starts an HTTP/SSE server at addr and blocks until ctx is cancelled.
func (s *Server) ServeSSE(ctx context.Context, addr string) error {
	handler := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return s.mcpServer }, nil)
	srv := &http.Server{Addr: addr, Handler: handler}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// askToRAGSystemInput is the JSON input the MCP framework deserialises for ask_to_rag_system.
type askToRAGSystemInput struct {
	Question string `json:"question"`
}

// askToRAGSystem retrieves relevant chunks from the knowledge base and returns them as LLM-ready context.
func (s *Server) askToRAGSystem(ctx context.Context, _ *mcp.CallToolRequest, in askToRAGSystemInput) (*mcp.CallToolResult, any, error) {
	log.Debug().Str("question", in.Question).Msg("ask_to_rag_system called")
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	retrieved, err := s.retriever.Retrieve(ctx, in.Question, s.topK)
	if err != nil {
		return nil, nil, fmt.Errorf("retrieve: %w", err)
	}
	var text string
	if retrieved == "" {
		log.Debug().Msg("no matching chunks found")
		text = "no relevant documents found"
	} else {
		text = "Use the following knowledge base context to answer the question.\n" +
			"Each excerpt is separated by \"---\".\n\n" + retrieved
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil, nil
}
