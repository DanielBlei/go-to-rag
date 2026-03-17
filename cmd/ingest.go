package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/chunker"
	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

var (
	chunkSize int
	overlap   int
	globPat   string
)

func init() {
	rootCmd.AddCommand(ingestCmd)
	ingestCmd.Flags().IntVar(&chunkSize, "chunk-size", 512, "chunk size in characters")
	ingestCmd.Flags().IntVar(&overlap, "overlap", 50, "overlap between chunks in characters")
	ingestCmd.Flags().StringVar(&globPat, "glob", "*.md", "glob pattern to match files")
}

const defaultIngestPath = "./seeds"

var ingestCmd = &cobra.Command{
	Use:   "ingest [path]",
	Short: "Embed documents from a directory into the vector store",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runIngest,
}

func runIngest(cmd *cobra.Command, args []string) error {
	ingestPath := defaultIngestPath
	if len(args) == 1 {
		ingestPath = args[0]
	}

	client, err := newOllamaClient()
	if err != nil {
		return fmt.Errorf("ollama init: %w", err)
	}

	if err := client.Validate(cmd.Context(), true, false); err != nil {
		if errors.Is(cmd.Context().Err(), context.Canceled) {
			return nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("ollama timed out — is it overloaded?")
		}
		return fmt.Errorf("ollama embed validation: %w", err)
	}

	store, err := vectorstore.NewSQLite(dbPath)
	if err != nil {
		return fmt.Errorf("open vector store: %w", err)
	}
	defer func() { _ = store.Close() }()

	pattern := filepath.Join(ingestPath, globPat)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob %q: %w", pattern, err)
	}
	if len(matches) == 0 {
		log.Warn().Str("pattern", pattern).Msg("no files matched")
		return nil
	}

	log.Info().Int("files", len(matches)).Str("pattern", pattern).Msg("starting ingest")

	var totalChunks int
	for _, path := range matches {
		if err := cmd.Context().Err(); err != nil {
			return nil
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("abs path %q: %w", path, err)
		}

		exists, err := store.HasSource(cmd.Context(), absPath)
		if err != nil {
			return fmt.Errorf("has source %q: %w", absPath, err)
		}
		if exists {
			log.Debug().Str("file", absPath).Msg("skipping already-ingested file")
			continue
		}

		chunks, err := chunker.ChunkFile(absPath, chunkSize, overlap)
		if err != nil {
			return fmt.Errorf("chunk %q: %w", absPath, err)
		}

		log.Debug().Str("file", absPath).Int("chunks", len(chunks)).Msg("embedding")

		var ingestErr error
		for _, c := range chunks {
			if err := cmd.Context().Err(); err != nil {
				return nil
			}

			emb, err := client.Embed(cmd.Context(), c.Text)
			if err != nil {
				if errors.Is(cmd.Context().Err(), context.Canceled) {
					return nil
				}
				ingestErr = fmt.Errorf("embed chunk %d of %q: %w", c.Index, absPath, err)
				break
			}

			if err := store.AddChunk(cmd.Context(), absPath, c.Text, emb, c.Index); err != nil {
				ingestErr = fmt.Errorf("store chunk %d of %q: %w", c.Index, absPath, err)
				break
			}
		}

		if ingestErr != nil {
			_ = store.DeleteSource(cmd.Context(), absPath)
			return ingestErr
		}

		totalChunks += len(chunks)
		log.Info().Str("file", absPath).Int("chunks", len(chunks)).Msg("ingested")
	}

	count, err := store.CountChunks(cmd.Context())
	if err != nil {
		log.Warn().Err(err).Msg("could not retrieve total chunk count")
	}
	log.Info().Int("new_chunks", totalChunks).Int("total_chunks", count).Msg("ingest complete")
	return nil
}
