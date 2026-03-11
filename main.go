package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/DanielBlei/go-to-rag/internal/ollama"
)

const (
	defaultHost       = "http://localhost:11434"
	defaultEmbedModel = "nomic-embed-text"
	defaultChatModel  = "qwen3.5:0.8b"
)

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

	client, err := ollama.New(*host, *embedModel, *chatModel)
	if err != nil {
		log.Fatal(err)
	}

	if err := client.Chat(context.Background(), prompt, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
