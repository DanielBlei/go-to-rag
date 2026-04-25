// Package ingest walks a directory, chunks matching files, embeds each chunk,
// and stores the embeddings in a vector store. It is the shared loop used by
// both the `ingest` CLI command and the eval harness.
package ingest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/DanielBlei/go-to-rag/internal/chunker"
	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

// Embedder produces an embedding vector for a single piece of text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Options controls how Run walks, filters, and stores files.
type Options struct {
	ChunkSize     int    // characters per chunk
	Overlap       int    // overlap between chunks in characters
	Glob          string // file-name glob (default "*.md" if empty)
	NoRecursive   bool   // when true, do not descend into subdirectories
	IncludeHidden bool   // when true, include hidden files and directories
	SkipExisting  bool   // when true, skip files whose source is already in the store

	// SourcePath maps a walked file path to the source identifier used as the
	// store key. CLI ingest uses filepath.Abs; eval uses corpus-relative paths.
	// Required.
	SourcePath func(path string) (string, error)

	// OnFile is called after each file is fully ingested (or skipped). Optional.
	OnFile func(source string, chunks int, skipped bool)
}

// Run walks root, ingests every file matching opts.Glob, and returns the list
// of source identifiers actually ingested (excluding skipped files) along with
// the total number of chunks added.
//
// On any error, the partially-ingested file is rolled back via DeleteSource so
// the store is never left with a half-embedded source.
func Run(
	ctx context.Context,
	store vectorstore.Store,
	embedder Embedder,
	root string,
	opts Options,
) (sources []string, totalChunks int, err error) {
	if opts.SourcePath == nil {
		return nil, 0, fmt.Errorf("ingest: Options.SourcePath is required")
	}
	glob := opts.Glob
	if glob == "" {
		glob = "*.md"
	}

	matches, err := walkFiles(root, glob, opts.NoRecursive, opts.IncludeHidden)
	if err != nil {
		return nil, 0, err
	}

	for _, path := range matches {
		if err := ctx.Err(); err != nil {
			return sources, totalChunks, err
		}

		source, err := opts.SourcePath(path)
		if err != nil {
			return sources, totalChunks, fmt.Errorf("source path %q: %w", path, err)
		}

		if opts.SkipExisting {
			exists, err := store.HasSource(ctx, source)
			if err != nil {
				return sources, totalChunks, fmt.Errorf("has source %q: %w", source, err)
			}
			if exists {
				log.Debug().Str("source", source).Msg("skipping already-ingested file")
				if opts.OnFile != nil {
					opts.OnFile(source, 0, true)
				}
				continue
			}
		}

		chunks, err := chunker.ChunkFile(path, opts.ChunkSize, opts.Overlap)
		if err != nil {
			return sources, totalChunks, fmt.Errorf("chunk %q: %w", path, err)
		}

		log.Debug().Str("source", source).Int("chunks", len(chunks)).Msg("embedding")

		var ingestErr error
		for _, c := range chunks {
			if err := ctx.Err(); err != nil {
				ingestErr = err
				break
			}
			emb, err := embedder.Embed(ctx, c.Text)
			if err != nil {
				ingestErr = fmt.Errorf("embed chunk %d of %q: %w", c.Index, source, err)
				break
			}
			if err := store.AddChunk(ctx, source, c.Text, emb, c.Index); err != nil {
				ingestErr = fmt.Errorf("store chunk %d of %q: %w", c.Index, source, err)
				break
			}
		}
		if ingestErr != nil {
			_ = store.DeleteSource(ctx, source)
			return sources, totalChunks, ingestErr
		}

		sources = append(sources, source)
		totalChunks += len(chunks)
		if opts.OnFile != nil {
			opts.OnFile(source, len(chunks), false)
		}
	}

	return sources, totalChunks, nil
}

// walkFiles returns all files under root whose base name matches glob.
// Hidden files and directories (names starting with '.') are skipped unless
// includeHidden is true. Subdirectories are skipped when noRecursive is true.
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
		if !includeHidden && isHidden {
			return nil
		}
		if matched, _ := filepath.Match(glob, base); matched {
			matches = append(matches, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk %q: %w", root, err)
	}
	return matches, nil
}
