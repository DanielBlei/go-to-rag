package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"

	"github.com/DanielBlei/go-to-rag/internal/rag"
)

const checkRAGKnowledgeBaseDescription = `
	Search the local knowledge base and return relevant context for a given question.
	Use this tool whenever the user asks about a topic that may be covered in the knowledge base.
	Pass the user's question as-is.
	The tool returns text context separated by "---" that you should use as primary context when answering.
	If the response says "no relevant documents found", answer from your own knowledge and you HAVE TO say so.`

const askToRAGSystemDescription = `
	Search the knowledge base and answer using an LLM.
	Use this when you want a synthesised answer, not raw context.
	The optional 'think' parameter controls reasoning tokens:
	"auto" (model default), "disabled" (no reasoning), "hidden" (reasons, tokens suppressed).
	Returns the answer. When think="auto" and the model emits reasoning,
	thinking tokens are included as a second content item.`

// Server wraps an MCP server with retrieval and chat capabilities.
type Server struct {
	retriever        rag.Pipeline
	chatServer       rag.ChatServer
	mcpServer        *mcp.Server
	topK             int
	defaultThinkMode rag.ThinkMode
}

// New creates an MCP server backed by the given Pipeline and ChatServer.
func New(retriever rag.Pipeline, chatServer rag.ChatServer, topK int, defaultThinkMode rag.ThinkMode) *Server {
	s := &Server{
		retriever:        retriever,
		chatServer:       chatServer,
		topK:             topK,
		defaultThinkMode: defaultThinkMode,
	}
	s.mcpServer = mcp.NewServer(&mcp.Implementation{Name: "go-to-rag", Version: "0.1.0"}, nil)
	s.registerTools()
	return s
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "check_rag_knowledge_base",
		Description: checkRAGKnowledgeBaseDescription,
	}, s.checkRAGKnowledgeBase)

	if s.chatServer != nil {
		mcp.AddTool(s.mcpServer, &mcp.Tool{
			Name:        "ask_to_rag_system",
			Description: askToRAGSystemDescription,
		}, s.askToRAGSystem)
	}
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

// checkRAGKnowledgeBaseInput is the JSON input the MCP framework deserialises for check_rag_knowledge_base.
type checkRAGKnowledgeBaseInput struct {
	Question string `json:"question"`
}

// checkRAGKnowledgeBase retrieves relevant chunks from the knowledge base and returns them as LLM-ready context.
func (s *Server) checkRAGKnowledgeBase(ctx context.Context, _ *mcp.CallToolRequest, in checkRAGKnowledgeBaseInput) (*mcp.CallToolResult, any, error) {
	log.Debug().Str("question", in.Question).Msg("check_rag_knowledge_base called")
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

// bufWriter accumulates answer and thinking tokens in memory.
type bufWriter struct {
	answer   strings.Builder
	thinking strings.Builder
}

func (b *bufWriter) Write(p []byte) (int, error)         { return b.answer.Write(p) }
func (b *bufWriter) WriteThinking(p []byte) (int, error) { return b.thinking.Write(p) }

// thinkModeFromString parses a think parameter string into a ThinkMode.
func thinkModeFromString(s string) rag.ThinkMode {
	switch s {
	case "disabled":
		return rag.ThinkDisabled
	case "hidden":
		return rag.ThinkHidden
	default:
		return rag.ThinkAuto
	}
}

// askToRAGSystemInput is the JSON input the MCP framework deserialises for ask_to_rag_system.
type askToRAGSystemInput struct {
	Question string `json:"question"`
	Think    string `json:"think"`
}

// askToRAGSystem retrieves context and generates an LLM answer with optional thinking.
func (s *Server) askToRAGSystem(ctx context.Context, _ *mcp.CallToolRequest, in askToRAGSystemInput) (*mcp.CallToolResult, any, error) {
	if in.Question == "" {
		return nil, nil, fmt.Errorf("question is required")
	}
	thinkMode := s.defaultThinkMode
	if in.Think != "" {
		thinkMode = thinkModeFromString(in.Think)
	}
	chatOpts := rag.ChatOptions{ThinkMode: thinkMode}
	log.Debug().Str("question", in.Question).Str("think", in.Think).Msg("ask_to_rag_system called")

	bw := &bufWriter{}
	if _, err := rag.Ask(ctx, s.retriever, s.chatServer, in.Question, s.topK, false, chatOpts, bw); err != nil {
		return nil, nil, fmt.Errorf("ask: %w", err)
	}
	content := []mcp.Content{&mcp.TextContent{Text: bw.answer.String()}}
	if bw.thinking.Len() > 0 {
		content = append(content, &mcp.TextContent{Text: "[thinking]\n" + bw.thinking.String()})
	}
	return &mcp.CallToolResult{Content: content}, nil, nil
}
