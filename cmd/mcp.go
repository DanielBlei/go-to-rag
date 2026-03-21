package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/mcpserver"
	"github.com/DanielBlei/go-to-rag/internal/ollama"
	"github.com/DanielBlei/go-to-rag/internal/rag"
)

var (
	mcpAddr string
	mcpTopK int
)

func init() {
	rootCmd.AddCommand(mcpCmd)
	addRAGFlags(mcpCmd)
	mcpCmd.Flags().StringVar(&mcpAddr, "addr", "", `HTTP/SSE listen address (e.g. ":8080"); omit for STDIO`)
	mcpCmd.Flags().IntVar(&mcpTopK, "top-k", 10, "number of chunks/top matches to retrieve from the vector store")
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server (STDIO by default, or SSE with --addr)",
	Args:  cobra.NoArgs,
	RunE:  runMCP,
}

func runMCP(cmd *cobra.Command, _ []string) error {
	client, err := ollama.New(host, embedModel, "")
	if err != nil {
		return fmt.Errorf("ollama init: %w", err)
	}

	store, err := openStore(cmd.Context(), dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	if err := client.Validate(cmd.Context(), true, false); err != nil {
		return fmt.Errorf("ollama validation: %w", err)
	}

	ragPipeline := rag.NewPipeline(client, store)
	srv := mcpserver.New(ragPipeline, mcpTopK)

	if mcpAddr != "" {
		return srv.ServeSSE(cmd.Context(), mcpAddr)
	}
	return srv.ServeStdio(cmd.Context())
}
