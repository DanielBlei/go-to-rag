package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/grpcserver"
	"github.com/DanielBlei/go-to-rag/internal/ollama"
	"github.com/DanielBlei/go-to-rag/internal/rag"
)

var (
	grpcAddr          string
	serveTopK         int
	serveModel        string
	serveWithFallback bool
)

func init() {
	rootCmd.AddCommand(serveCmd)
	addRAGFlags(serveCmd)
	serveCmd.Flags().StringVar(&grpcAddr, "grpc-addr", ":50051", "gRPC listen address")
	serveCmd.Flags().IntVar(&serveTopK, "top-k", 10, "default number of chunks to retrieve")
	serveCmd.Flags().StringVar(&serveModel, "model", defaultChatModel, "Ollama chat model")
	serveCmd.Flags().BoolVar(&serveWithFallback, "with-fallback", false, "allow the model to answer from its own knowledge when context is missing")
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the gRPC server",
	Args:  cobra.NoArgs,
	RunE:  runServe,
}

func runServe(cmd *cobra.Command, _ []string) error {
	client, err := ollama.New(host, embedModel, serveModel)
	if err != nil {
		return fmt.Errorf("ollama init: %w", err)
	}

	store, err := openStore(cmd.Context(), dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	if err := client.Validate(cmd.Context(), true, true); err != nil {
		return fmt.Errorf("ollama validation: %w", err)
	}

	ragPipeline := rag.NewPipeline(client, store)
	// client satisfies both rag.Embedder (via ragPipeline) and grpcserver.ChatServer.
	srv := grpcserver.New(ragPipeline, client, serveTopK, serveWithFallback)

	return srv.Serve(cmd.Context(), grpcAddr)
}
