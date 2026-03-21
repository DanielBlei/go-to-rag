package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/ollama"
	"github.com/DanielBlei/go-to-rag/internal/rag"
	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

const fallbackSystemPrompt = `You are a helpful assistant with deep knowledge of software and AI systems.
Answer questions clearly and concisely.

Rules:
- Use the provided context as your primary source.
- You may supplement the context with your own knowledge to give a more complete answer.
- When adding information not found in the context, you HAVE TO inform the user:
  "Note: supplementing answer with my own knowledge."
- Never fabricate facts or sources.`

var (
	chatModel    string
	withFallback bool
	topK         int
)

func init() {
	rootCmd.AddCommand(askCmd)
	addRAGFlags(askCmd)
	askCmd.Flags().StringVar(&chatModel, "model", defaultChatModel, "Ollama chat model")
	askCmd.Flags().BoolVar(&withFallback, "with-fallback", false, "allow the model to answer from its own knowledge when context is missing")
	askCmd.Flags().IntVar(&topK, "top-k", 5, "number of chunks/top matches to retrieve from the vector store")
}

var askCmd = &cobra.Command{
	Use:   "ask <prompt>",
	Short: "Ask a single question and get a response",
	Args:  cobra.ExactArgs(1),
	RunE:  runAsk,
}

func runAsk(cmd *cobra.Command, args []string) error {
	prompt := args[0]

	log.Debug().Str("model", chatModel).Str("embed-model", embedModel).
		Str("host", host).Str("db", dbPath).Int("top-k", topK).
		Bool("with-fallback", withFallback).Msg("initializing ask")

	client, err := ollama.New(host, embedModel, chatModel)
	if err != nil {
		return fmt.Errorf("ollama init: %w", err)
	}

	store, storeErr := openStore(cmd.Context(), dbPath)
	if storeErr != nil {
		log.Warn().Err(storeErr).Msg("falling back to model knowledge")
	} else {
		defer func() { _ = store.Close() }()
	}

	checkEmbed := store != nil
	if err := client.Validate(cmd.Context(), checkEmbed, true); err != nil {
		if errors.Is(cmd.Context().Err(), context.Canceled) {
			return nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("ollama timed out, is it overloaded")
		}
		return fmt.Errorf("ollama validation: %w", err)
	}

	var ragPipelineContext string
	if store != nil {
		storeContextRetrieved, err := rag.Retrieve(cmd.Context(), prompt, topK, client, store)
		if err != nil {
			return fmt.Errorf("retrieve failure: %w", err)
		}
		if storeContextRetrieved == "" {
			log.Warn().Msg("no matching chunks found")
		}
		ragPipelineContext = storeContextRetrieved
	}

	log.Debug().Str("prompt", prompt).Bool("rag", ragPipelineContext != "").Msg("user input")

	var sysPrompt string
	if withFallback || ragPipelineContext == "" {
		sysPrompt = fallbackSystemPrompt
	}

	chatErr := client.Chat(cmd.Context(), sysPrompt, ragPipelineContext, prompt, os.Stdout)
	if chatErr != nil {
		if errors.Is(cmd.Context().Err(), context.Canceled) {
			_, _ = fmt.Fprintln(os.Stdout)
			return nil
		}
		if errors.Is(chatErr, context.DeadlineExceeded) {
			return fmt.Errorf("ollama chat timed out, consider increasing chatTimeout")
		}
		return fmt.Errorf("chat: %w", chatErr)
	}
	_, _ = fmt.Fprintln(os.Stdout)
	return nil
}

// openStore attempts to open the vector store at path. Returns an error if the
// store is unavailable for any reason, caller falls back to model knowledge.
func openStore(ctx context.Context, path string) (vectorstore.Store, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("store not found at %q, run 'ingest' to build one, or repoint --db", path)
	}

	store, err := vectorstore.NewSQLite(path)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	count, err := store.CountChunks(ctx)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("count chunks: %w", err)
	}
	if count == 0 {
		_ = store.Close()
		return nil, fmt.Errorf("store is empty, run 'ingest' to populate it and/or add more docs to the knowledge base")
	}

	return store, nil
}
