package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(askCmd)
}

var askCmd = &cobra.Command{
	Use:   "ask <prompt>",
	Short: "Ask a single question and get a response",
	Args:  cobra.ExactArgs(1),
	RunE:  runAsk,
}

func runAsk(cmd *cobra.Command, args []string) error {
	client, err := newOllamaClient()
	if err != nil {
		return fmt.Errorf("ollama init: %w", err)
	}

	if err := client.Validate(cmd.Context(), false, true); err != nil {
		if errors.Is(cmd.Context().Err(), context.Canceled) {
			return nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("ollama timed out — is it overloaded?")
		}
		return fmt.Errorf("ollama chat validation: %w", err)
	}

	log.Debug().Str("prompt", args[0]).Msg("user input")

	if err := client.Chat(cmd.Context(), args[0], os.Stdout); err != nil {
		if errors.Is(cmd.Context().Err(), context.Canceled) {
			return nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("ollama chat timed out, consider increasing chatTimeout")
		}
		return fmt.Errorf("chat: %w", err)
	}

	_, _ = fmt.Fprintln(os.Stdout)
	return nil
}
