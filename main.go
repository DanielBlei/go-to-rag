package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/DanielBlei/go-to-rag/internal/ollama"
)

const (
	defaultHost       = "http://localhost:11434"
	defaultEmbedModel = "nomic-embed-text"
	defaultChatModel  = "qwen3.5:0.8b"
)

// withSignalCancel returns a context that is canceled when SIGINT or SIGTERM is
// received, logging the signal before canceling.
func withSignalCancel(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigCh:
			log.Printf("received signal %s, shutting down", sig)
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
	flag.Parse()

	prompt := flag.Arg(0)
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "usage: go-to-rag [flags] <prompt>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	ctx, cancel := withSignalCancel(context.Background())
	defer cancel()

	client, err := ollama.New(ctx, *host, *embedModel, *chatModel)
	if err != nil {
		if ctx.Err() == context.Canceled {
			os.Exit(0) // clean shutdown via signal
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Fatal("ollama timed out — is it overloaded?")
		}
		log.Fatal(err)
	}

	if err := client.Chat(ctx, prompt, os.Stdout); err != nil {
		if ctx.Err() == context.Canceled {
			os.Exit(0) // clean shutdown via signal
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Fatal("ollama chat timed out, consider increasing chatTimeout...")
		}
		log.Fatal(err)
	}
}