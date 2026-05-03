package cmd

import (
	"net"

	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/grpcserver"
	"github.com/DanielBlei/go-to-rag/internal/inference"
	"github.com/DanielBlei/go-to-rag/internal/rag"
)

var (
	grpcAddr          string
	serveTopK         int
	serveModel        string
	serveWithFallback bool

	// grpcListener may be set by tests to pass a pre-bound listener to runServe,
	// eliminating the listen-close-rebind race. Nil in normal operation.
	grpcListener net.Listener
)

func init() {
	rootCmd.AddCommand(serveCmd)
	addRAGFlags(serveCmd)
	serveCmd.Flags().StringVar(&grpcAddr, "grpc-addr", ":50051", "gRPC listen address")
	serveCmd.Flags().IntVar(&serveTopK, "top-k", 10, "default number of chunks to retrieve")
	serveCmd.Flags().StringVar(&serveModel, "model", defaultChatModel, "Ollama chat model")
	serveCmd.Flags().
		BoolVar(&serveWithFallback, "with-fallback", false, "allow the model to answer from its own knowledge when context is missing")
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the gRPC server",
	Args:  cobra.NoArgs,
	RunE:  runServe,
}

func runServe(cmd *cobra.Command, _ []string) error {
	store, err := openStore(cmd.Context(), dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	embedder, chatServer, err := inference.Resolve(cmd.Context(), inference.ResolveConfig{
		Provider:   inferenceProvider,
		Host:       host,
		EmbedHost:  embedHost,
		EmbedModel: embedModel,
		ChatModel:  serveModel,
		APIKey:     apiKey,
		CheckEmbed: true,
		CheckChat:  true,
	})
	if err != nil {
		return err
	}

	ragPipeline := rag.NewPipeline(embedder, store)
	// Server is stateless: clients dictate think_mode via gRPC requests.
	// When clients omit think_mode, the server uses ThinkAuto (model default).
	srv := grpcserver.New(ragPipeline, chatServer, serveTopK, serveWithFallback, rag.ThinkAuto)

	if grpcListener != nil {
		return srv.ServeListener(cmd.Context(), grpcListener)
	}
	return srv.Serve(cmd.Context(), grpcAddr)
}
