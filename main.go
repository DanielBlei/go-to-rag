package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"

	"github.com/DanielBlei/go-to-rag/internal/logger"
	"github.com/DanielBlei/go-to-rag/internal/ollama"
)

var log zerolog.Logger

const (
	defaultHost       = "http://localhost:11434"
	defaultEmbedModel = "nomic-embed-text"
	defaultChatModel  = "llama3.2:1b"
)

// withSignalCancel returns a context that is canceled when SIGINT or SIGTERM is received.
// informing the shutdown process
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

func main() {
	host := flag.String("host", defaultHost, "Ollama host URL")
	embedModel := flag.String("embed-model", defaultEmbedModel, "Ollama embedding model")
	chatModel := flag.String("model", defaultChatModel, "Ollama chat model")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	log = logger.New(*debug)

	prompt := flag.Arg(0)
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "usage: go-to-rag [flags] <prompt>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	ctx, cancel := withSignalCancel(context.Background())
	defer cancel()

	client, err := ollama.New(*host, *embedModel, *chatModel)
	if err != nil {
		log.Fatal().Err(err).Msg("ollama init failed")
	}

	if err := client.Validate(ctx, false, true); err != nil {
		if ctx.Err() == context.Canceled {
			os.Exit(0)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Fatal().Msg("ollama timed out — is it overloaded?")
		}
		log.Fatal().Err(err).Msg("ollama init failed")
	}

	log.Debug().Str("prompt", prompt).Msg("user input")

	if err := client.Chat(ctx, prompt, os.Stdout); err != nil {
		if ctx.Err() == context.Canceled {
			os.Exit(0)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Fatal().Msg("ollama chat timed out, consider increasing chatTimeout...")
		}
		log.Fatal().Err(err).Msg("chat failed")
	}
	_, _ = fmt.Fprintln(os.Stdout)
}
