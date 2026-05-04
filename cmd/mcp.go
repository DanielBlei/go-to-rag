package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/inference"
	"github.com/DanielBlei/go-to-rag/internal/mcpserver"
	"github.com/DanielBlei/go-to-rag/internal/rag"
)

var (
	mcpAddr          string
	mcpTopK          int
	mcpChatModel     string
	mcpThinkMode     = rag.ThinkHidden
	mcpConfThreshold float64
)

func init() {
	rootCmd.AddCommand(mcpCmd)
	addRAGFlags(mcpCmd)
	mcpCmd.Flags().
		StringVar(&mcpAddr, "addr", "", `HTTP/SSE listen address (e.g. ":8080"); omit for STDIO`)
	mcpCmd.Flags().
		IntVar(&mcpTopK, "top-k", 10, "number of chunks/top matches to retrieve from the vector store")
	mcpCmd.Flags().
		StringVar(&mcpChatModel, "chat-model", "", "chat model name; required to enable the ask_to_rag_system chat tool")
	mcpCmd.Flags().
		Var(&thinkModeFlag{val: &mcpThinkMode}, "think", "default thinking mode: auto, disabled, or hidden")
	mcpCmd.Flags().
		Float64Var(&mcpConfThreshold, "confidence-threshold", 0.5, "cosine similarity score below which retrieved chunks are flagged as low-confidence (0.0–1.0)")
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server (STDIO by default, or SSE with --addr)",
	Args:  cobra.NoArgs,
	RunE:  runMCP,
}

func runMCP(cmd *cobra.Command, _ []string) error {
	store, err := openStore(cmd.Context(), dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	checkChat := mcpChatModel != ""
	embedder, chatServer, err := inference.Resolve(cmd.Context(), inference.ResolveConfig{
		Provider:   inferenceProvider,
		ChatHost:   host,
		EmbedHost:  embedHost,
		EmbedModel: embedModel,
		ChatModel:  mcpChatModel,
		APIKey:     apiKey,
		CheckEmbed: true,
		CheckChat:  checkChat,
	})
	if err != nil {
		return err
	}

	ragPipeline := rag.NewPipeline(embedder, store)
	srv := mcpserver.New(ragPipeline, chatServer, mcpTopK, mcpThinkMode, mcpConfThreshold)

	if mcpAddr != "" {
		return srv.ServeSSE(cmd.Context(), mcpAddr)
	}
	return srv.ServeStdio(cmd.Context())
}
