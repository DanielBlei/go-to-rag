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

var (
	chatModel    string
	withFallback bool
	topK         int
	thinkMode    rag.ThinkMode
)

// thinkModeFlag implements pflag.Value for the --think enum.
type thinkModeFlag struct {
	val *rag.ThinkMode
}

func (f *thinkModeFlag) String() string {
	switch *f.val {
	case rag.ThinkDisabled:
		return "disabled"
	case rag.ThinkHidden:
		return "hidden"
	default:
		return "auto"
	}
}

func (f *thinkModeFlag) Set(s string) error {
	switch s {
	case "auto":
		*f.val = rag.ThinkAuto
	case "disabled":
		*f.val = rag.ThinkDisabled
	case "hidden":
		*f.val = rag.ThinkHidden
	default:
		return fmt.Errorf("invalid --think value %q, must be auto, disabled, or hidden", s)
	}
	return nil
}

func (f *thinkModeFlag) Type() string {
	return "ThinkMode"
}

func init() {
	rootCmd.AddCommand(askCmd)
	addRAGFlags(askCmd)
	askCmd.Flags().StringVar(&chatModel, "model", defaultChatModel, "Ollama chat model")
	askCmd.Flags().
		BoolVar(&withFallback, "with-fallback", false, "allow the model to answer from its own knowledge when context is missing")
	askCmd.Flags().
		IntVar(&topK, "top-k", defaultTopK, "number of chunks/top matches to retrieve from the vector store")
	askCmd.Flags().Var(&thinkModeFlag{val: &thinkMode}, "think",
		"control thinking tokens: auto (model default), disabled (no thinking), or hidden (model thinks but output suppressed)")
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
		Bool("with-fallback", withFallback).Int("think", int(thinkMode)).Msg("initializing ask")

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
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("ollama timed out, is it overloaded")
		}
		return fmt.Errorf("ollama validation: %w", err)
	}

	var chatErr error
	chatOpts := rag.ChatOptions{ThinkMode: thinkMode}
	writer := ollama.NewTerminalWriter(os.Stdout)
	if store != nil {
		pipeline := rag.NewPipeline(client, store)
		contextBlock, err := rag.Ask(
			cmd.Context(),
			pipeline,
			client,
			prompt,
			topK,
			withFallback,
			chatOpts,
			writer,
		)
		if err != nil {
			chatErr = err
		}
		if contextBlock == "" {
			log.Warn().Msg("no matching chunks found")
		}
		log.Debug().Str("prompt", prompt).Bool("rag", contextBlock != "").Msg("user input")
	} else {
		log.Debug().Str("prompt", prompt).Bool("rag", false).Msg("user input")
		chatErr = client.Chat(cmd.Context(), rag.FallbackSystemPrompt, "", prompt, chatOpts, writer)
	}

	if chatErr != nil {
		// Move the cursor to a new line if the user interrupted mid-stream.
		// context.Canceled itself is handled in Execute().
		// This guard exists only to flush a trailing newline before the clean exit.
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
		return nil, fmt.Errorf(
			"store not found at %q, run 'ingest' to build one, or repoint --db",
			path,
		)
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
		return nil, fmt.Errorf(
			"store is empty, run 'ingest' to populate it and/or add more docs to the knowledge base",
		)
	}

	return store, nil
}
