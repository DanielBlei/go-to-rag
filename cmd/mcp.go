package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/mcpserver"
	"github.com/DanielBlei/go-to-rag/internal/ollama"
	"github.com/DanielBlei/go-to-rag/internal/rag"
)

var (
	mcpAddr      string
	mcpTopK      int
	mcpChatModel string
	mcpThinkMode = rag.ThinkHidden
)

func init() {
	rootCmd.AddCommand(mcpCmd)
	addRAGFlags(mcpCmd)
	mcpCmd.Flags().
		StringVar(&mcpAddr, "addr", "", `HTTP/SSE listen address (e.g. ":8080"); omit for STDIO`)
	mcpCmd.Flags().
		IntVar(&mcpTopK, "top-k", 10, "number of chunks/top matches to retrieve from the vector store")
	mcpCmd.Flags().
		StringVar(&mcpChatModel, "model", "", "Ollama chat model; required to enable the ask_to_rag_system chat tool")
	mcpCmd.Flags().
		Var(&thinkModeFlag{val: &mcpThinkMode}, "think", "default thinking mode: auto, disabled, or hidden")
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server (STDIO by default, or SSE with --addr)",
	Args:  cobra.NoArgs,
	RunE:  runMCP,
}

func runMCP(cmd *cobra.Command, _ []string) error {
	client, err := ollama.New(host, embedModel, mcpChatModel)
	if err != nil {
		return fmt.Errorf("ollama init: %w", err)
	}

	store, err := openStore(cmd.Context(), dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	checkChat := mcpChatModel != ""
	if err := client.Validate(cmd.Context(), true, checkChat); err != nil {
		return fmt.Errorf("ollama validation: %w", err)
	}

	ragPipeline := rag.NewPipeline(client, store)
	var chatServer rag.ChatServer
	if mcpChatModel != "" {
		chatServer = client
	}
	srv := mcpserver.New(ragPipeline, chatServer, mcpTopK, mcpThinkMode)

	if mcpAddr != "" {
		return srv.ServeSSE(cmd.Context(), mcpAddr)
	}
	return srv.ServeStdio(cmd.Context())
}
