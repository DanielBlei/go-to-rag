package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/chunker"
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
	ingestCmd.Flags().BoolVar(&noRecursive, "no-recursive", false, "only match files in the root directory, do not recurse")
	ingestCmd.Flags().BoolVar(&includeHidden, "include-hidden", false, "include hidden files and directories (names starting with .)")
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

	matches, err := walkFiles(ingestPath, globPat, noRecursive, includeHidden)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		log.Warn().Str("path", ingestPath).Str("glob", globPat).Msg("no files matched")
		return nil
	}

	log.Info().Int("files", len(matches)).Str("path", ingestPath).Str("glob", globPat).Msg("starting ingest")

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

// walkFiles returns all files under root whose base name matches glob.
// Hidden files and directories (names starting with '.') are skipped unless includeHidden is true.
// Subdirectories are skipped when noRecursive is true.
// Symlinked directories are never followed; a warning is logged if one is detected.
func walkFiles(root, glob string, noRecursive, includeHidden bool) ([]string, error) {
	if _, err := filepath.Match(glob, ""); err != nil {
		return nil, fmt.Errorf("invalid glob pattern %q: %w", glob, err)
	}
	var matches []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// WalkDir reports symlinks but does not follow them into directories.
		// Detect symlinked dirs and warn — their contents are never walked.
		if d.Type()&fs.ModeSymlink != 0 {
			if fi, serr := os.Stat(path); serr == nil && fi.IsDir() {
				log.Warn().Str("path", path).Msg("skipping symlinked directory")
			}
			return nil
		}
		base := filepath.Base(path)
		isHidden := len(base) > 1 && base[0] == '.'
		if d.IsDir() {
			if path == root {
				return nil
			}
			if !includeHidden && isHidden {
				return fs.SkipDir
			}
			if noRecursive {
				return fs.SkipDir
			}
			return nil
		}
		// file
		if !includeHidden && isHidden {
			return nil
		}
		if matched, _ := filepath.Match(glob, filepath.Base(path)); matched {
			matches = append(matches, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk %q: %w", root, err)
	}
	return matches, nil
}
