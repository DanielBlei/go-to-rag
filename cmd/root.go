package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/logger"
)

var (
	host              = defaultHost
	embedHost         string
	embedModel        string
	dbPath            string
	debug             bool
	log               zerolog.Logger
	inferenceProvider = "ollama"
	apiKey            string
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

// hostFlag implements pflag.Value for URL flags.
// Validates that the value is a well-formed http/https URL; for bare IP addresses
// (no hostname) an explicit port is required. An empty value is accepted — it
// means "default to --chat-host" (used by --embed-host).
type hostFlag struct{ val *string }

func (f *hostFlag) String() string { return *f.val }
func (f *hostFlag) Type() string   { return "url" }
func (f *hostFlag) Set(s string) error {
	if s == "" {
		*f.val = s
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%q must be a full URL with scheme and host (e.g. http://localhost:8000)", s)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%q: scheme must be http or https", s)
	}
	if net.ParseIP(u.Hostname()) != nil && u.Port() == "" {
		return fmt.Errorf("%q: bare IP address requires an explicit port (e.g. http://%s:8000)", s, u.Hostname())
	}
	*f.val = s
	return nil
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
	cmd.Flags().Var(&hostFlag{val: &host}, "chat-host", "URL for the chat/inference server")
	cmd.Flags().
		Var(&hostFlag{val: &embedHost}, "embed-host", "URL for the embedding server; defaults to --chat-host when empty")
	cmd.Flags().StringVar(&embedModel, "embed-model", defaultEmbedModel, "embedding model name")
	cmd.Flags().StringVar(&dbPath, "db", defaultDBPath, "path to the vector store database")
	cmd.Flags().Var(&inferenceFlag{val: &inferenceProvider}, "inference", "inference provider: ollama or vllm")
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
