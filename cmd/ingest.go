package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/ingest"
	"github.com/DanielBlei/go-to-rag/internal/ollama"
	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

var (
	chunkSize     int
	overlap       int
	globPat       string
	noRecursive   bool
	includeHidden bool
)

func init() {
	rootCmd.AddCommand(ingestCmd)
	addRAGFlags(ingestCmd)
	ingestCmd.Flags().IntVar(&chunkSize, "chunk-size", 512, "chunk size in characters")
	ingestCmd.Flags().IntVar(&overlap, "overlap", 100, "overlap between chunks in characters")
	ingestCmd.Flags().StringVar(&globPat, "glob", "*.md", "glob pattern to match files")
	ingestCmd.Flags().
		BoolVar(&noRecursive, "no-recursive", false, "only match files in the root directory, do not recurse")
	ingestCmd.Flags().
		BoolVar(&includeHidden, "include-hidden", false, "include hidden files and directories (names starting with .)")
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

	client, err := ollama.New(host, embedModel, "")
	if err != nil {
		return fmt.Errorf("ollama init: %w", err)
	}

	if err := client.Validate(cmd.Context(), true, false); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("ollama timed out, is it overloaded")
		}
		return fmt.Errorf("ollama embed validation: %w", err)
	}

	store, err := vectorstore.NewSQLite(dbPath)
	if err != nil {
		return fmt.Errorf("open vector store: %w", err)
	}
	defer func() { _ = store.Close() }()

	log.Info().
		Str("path", ingestPath).
		Str("glob", globPat).
		Msg("starting ingest")

	sources, totalChunks, err := ingest.Run(cmd.Context(), store, client, ingestPath, ingest.Options{
		ChunkSize:     chunkSize,
		Overlap:       overlap,
		Glob:          globPat,
		NoRecursive:   noRecursive,
		IncludeHidden: includeHidden,
		SkipExisting:  true,
		SourcePath:    filepath.Abs,
		OnFile: func(source string, chunks int, skipped bool) {
			if skipped {
				log.Debug().Str("file", source).Msg("skipping already-ingested file")
				return
			}
			log.Info().Str("file", source).Int("chunks", chunks).Msg("ingested")
		},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		return err
	}
	if len(sources) == 0 && totalChunks == 0 {
		log.Warn().Str("path", ingestPath).Str("glob", globPat).Msg("no files matched or all already ingested")
	}

	count, err := store.CountChunks(cmd.Context())
	if err != nil {
		log.Warn().Err(err).Msg("could not retrieve total chunk count")
	}
	log.Info().Int("new_chunks", totalChunks).Int("total_chunks", count).Msg("ingest complete")
	return nil
}
