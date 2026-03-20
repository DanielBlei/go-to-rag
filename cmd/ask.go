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
- Prefer information from the provided context when available.
- If the context is not helpful or missing, answer using your own knowledge.
- When using your own knowledge instead of the context, inform the user:
  "Note: I'm answering from my own references, not from the provided context."
- Never fabricate facts or sources.`

var (
	chatModel    string
	withFallback bool
)

func init() {
	rootCmd.AddCommand(askCmd)
	addRAGFlags(askCmd)
	askCmd.Flags().StringVar(&chatModel, "model", defaultChatModel, "Ollama chat model")
	askCmd.Flags().BoolVar(&withFallback, "with-fallback", false, "allow the model to answer from its own knowledge when context is missing")
}

var askCmd = &cobra.Command{
	Use:   "ask <prompt>",
	Short: "Ask a single question and get a response",
	Args:  cobra.ExactArgs(1),
	RunE:  runAsk,
}

func runAsk(cmd *cobra.Command, args []string) error {
	prompt := args[0]

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
			return fmt.Errorf("ollama timed out, is it overloaded?")
		}
		return fmt.Errorf("ollama validation: %w", err)
	}

	var ragPipelineContext string
	if store != nil {
		storeContextRetrieved, err := rag.Retrieve(cmd.Context(), prompt, 5, client, store)
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

	var chatErr error
	if ragPipelineContext != "" {
		log.Debug().Str("rag pipeline context", ragPipelineContext).Msg("injecting retrieved context")
		chatErr = client.AskWithContext(cmd.Context(), sysPrompt, ragPipelineContext, prompt, os.Stdout)
	} else {
		chatErr = client.Chat(cmd.Context(), sysPrompt, prompt, os.Stdout)
	}

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
