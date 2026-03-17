package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/logger"
	"github.com/DanielBlei/go-to-rag/internal/ollama"
)

var (
	host       string
	embedModel string
	chatModel  string
	debug      bool
	log        zerolog.Logger
)

const (
	defaultHost       = "http://localhost:11434"
	defaultEmbedModel = "nomic-embed-text"
	defaultChatModel  = "llama3.2:1b"
)

var rootCmd = &cobra.Command{
	Use:     "go-to-rag",
	Short:   "A local RAG engine powered by Ollama",
	Version: "0.1.0",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		log = logger.New(debug)
		return nil
	},
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&host, "host", defaultHost, "Ollama host URL")
	rootCmd.PersistentFlags().StringVar(&chatModel, "model", defaultChatModel, "Ollama chat model")
	rootCmd.PersistentFlags().StringVar(&embedModel, "embed-model", defaultEmbedModel, "Ollama embedding model")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
}

func newOllamaClient() (*ollama.Client, error) {
	return ollama.New(host, embedModel, chatModel)
}

// withSignalCancel returns a context that is cancelled when SIGINT or SIGTERM is received.
func withSignalCancel(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigCh:
			log.Info().Str("signal", sig.String()).Msg("shutting down")
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	return ctx, cancel
}

// Execute runs the root command with signal-aware context.
func Execute() error {
	ctx, cancel := withSignalCancel(context.Background())
	defer cancel()
	return rootCmd.ExecuteContext(ctx)
}
