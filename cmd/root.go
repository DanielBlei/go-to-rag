package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/logger"
)

var (
	host             string
	embedModel       string
	dbPath           string
	debug            bool
	log              zerolog.Logger
	inferenceBackend = "ollama"
	apiKey           string
)

// inferenceFlag implements pflag.Value for the --inference enum.
type inferenceFlag struct{ val *string }

func (f *inferenceFlag) String() string { return *f.val }
func (f *inferenceFlag) Type() string   { return "backend" }
func (f *inferenceFlag) Set(s string) error {
	switch s {
	case "ollama", "vllm":
		*f.val = s
		return nil
	default:
		return fmt.Errorf("invalid --inference value %q, must be ollama or vllm", s)
	}
}

const (
	defaultHost       = "http://localhost:11434"
	defaultEmbedModel = "mxbai-embed-large:latest"
	defaultChatModel  = "qwen3:1.7b"
	defaultDBPath     = "./data/index.db"
	defaultTopK       = 10
	defaultChunkSize  = 512
	defaultOverlap    = 100
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
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
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

// addRAGFlags registers flags shared by commands that talk to the inference backend and vector store.
// todo: move to persistent flags; when calling Execute() more than once, e.g. integration tests, there is a stale state risk.
func addRAGFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&host, "host", defaultHost, "inference backend host URL")
	cmd.Flags().StringVar(&embedModel, "embed-model", defaultEmbedModel, "embedding model name")
	cmd.Flags().StringVar(&dbPath, "db", defaultDBPath, "path to the vector store database")
	cmd.Flags().Var(&inferenceFlag{val: &inferenceBackend}, "inference", "inference backend: ollama or vllm")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "bearer token for backend auth (vLLM production, Ollama cloud)")
}

// Execute runs the root command with signal-aware context.
func Execute() error {
	ctx, cancel := withSignalCancel(context.Background())
	defer cancel()
	err := rootCmd.ExecuteContext(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
