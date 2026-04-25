package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/DanielBlei/go-to-rag/internal/eval"
	"github.com/DanielBlei/go-to-rag/internal/ollama"
)

const (
	defaultEvalDataset = "internal/eval/testdata/golden.v1.json"
	defaultEvalCorpus  = "internal/eval/testdata/corpus"
	defaultEvalWorkDir = "./.eval-cache"
)

var (
	evalHost       string
	evalEmbedModel string
	evalDataset    string
	evalCorpus     string
	evalTopK       int
	evalOutput     string
	evalFormat     string
	evalChunkSize  int
	evalOverlap    int
	evalReuseDB    bool
	evalWorkDir    string
)

func init() {
	rootCmd.AddCommand(evalCmd)
	evalCmd.Flags().StringVar(&evalHost, "host", defaultHost, "Ollama host URL")
	evalCmd.Flags().StringVar(&evalEmbedModel, "embed-model", defaultEmbedModel, "Ollama embedding model")
	evalCmd.Flags().StringVar(&evalDataset, "dataset", defaultEvalDataset, "path to golden.v1.json")
	evalCmd.Flags().StringVar(&evalCorpus, "corpus", defaultEvalCorpus, "path to the frozen corpus directory")
	evalCmd.Flags().IntVar(&evalTopK, "top-k", 5, "top-k chunks to retrieve per query")
	evalCmd.Flags().StringVar(&evalOutput, "output", "", "output path; stdout when empty")
	evalCmd.Flags().StringVar(&evalFormat, "format", "json", "output format: json or text")
	evalCmd.Flags().IntVar(&evalChunkSize, "chunk-size", 512, "chunk size in characters")
	evalCmd.Flags().IntVar(&evalOverlap, "overlap", 100, "overlap between chunks in characters")
	evalCmd.Flags().BoolVar(&evalReuseDB, "reuse-db", false,
		"persist the embedded corpus to <work-dir>/run.db and reuse it on the next run")
	evalCmd.Flags().StringVar(&evalWorkDir, "work-dir", defaultEvalWorkDir,
		"directory for ephemeral build dirs and the optional reuse cache")
}

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Run retrieval-quality evaluation against a frozen corpus and golden dataset",
	RunE:  runEval,
}

func runEval(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	dataset, err := eval.LoadGolden(evalDataset)
	if err != nil {
		return fmt.Errorf("load dataset: %w", err)
	}
	datasetHash, err := hashFile(evalDataset)
	if err != nil {
		return fmt.Errorf("hash dataset: %w", err)
	}

	corpusHash, err := hashCorpus(evalCorpus)
	if err != nil {
		return fmt.Errorf("hash corpus: %w", err)
	}

	client, err := ollama.New(evalHost, evalEmbedModel, "")
	if err != nil {
		return fmt.Errorf("ollama init: %w", err)
	}
	if err := client.Validate(ctx, true, false); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("ollama timed out, is it overloaded")
		}
		return fmt.Errorf("ollama embed validation: %w", err)
	}
	digest, err := client.EmbedModelDigest(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("could not fetch embed-model digest")
	}

	meta := eval.HermeticMeta{
		EmbedModelDigest: digest,
		CorpusHash:       corpusHash,
		ChunkSize:        evalChunkSize,
		Overlap:          evalOverlap,
	}

	log.Info().
		Str("corpus", evalCorpus).
		Str("dataset", evalDataset).
		Int("queries", len(dataset)).
		Int("top_k", evalTopK).
		Str("work_dir", evalWorkDir).
		Bool("reuse_db", evalReuseDB).
		Msg("starting eval")

	buildStart := time.Now()
	setup, err := eval.BuildHermetic(ctx, client, eval.HermeticOptions{
		CorpusDir: evalCorpus,
		ChunkSize: evalChunkSize,
		Overlap:   evalOverlap,
		WorkDir:   evalWorkDir,
		Reuse:     evalReuseDB,
		Meta:      meta,
	})
	if err != nil {
		return fmt.Errorf("build pipeline: %w", err)
	}
	defer setup.Cleanup()
	log.Info().
		Dur("elapsed", time.Since(buildStart).Round(time.Millisecond)).
		Int("sources", len(setup.Sources)).
		Bool("reused", evalReuseDB).
		Msg("pipeline ready")

	runStart := time.Now()
	report, err := eval.Run(ctx, setup.Pipeline, setup.Sources, dataset, evalTopK)
	log.Info().
		Dur("elapsed", time.Since(runStart).Round(time.Millisecond)).
		Int("queries", len(dataset)).
		Msg("retrieval done")
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("run: %w", err)
	}

	report.Config = &eval.Config{
		EmbedModel:       evalEmbedModel,
		EmbedModelDigest: digest,
		ChunkSize:        evalChunkSize,
		Overlap:          evalOverlap,
		TopK:             evalTopK,
		CorpusPath:       evalCorpus,
		CorpusHash:       corpusHash,
		DatasetPath:      evalDataset,
		DatasetHash:      datasetHash,
		RunStartedAt:     time.Now().UTC(),
	}

	w, closeFn, err := openOutput(evalOutput)
	if err != nil {
		return fmt.Errorf("open output: %w", err)
	}
	defer closeFn()

	switch evalFormat {
	case "json":
		if err := report.WriteJSON(w); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
	case "text":
		if err := report.WriteText(w); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
	default:
		return fmt.Errorf("unknown --format %q (want json or text)", evalFormat)
	}
	return nil
}

func openOutput(path string) (io.Writer, func(), error) {
	if path == "" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// hashCorpus returns a stable SHA-256 over (relpath, content) pairs of every
// .md file under corpusDir, sorted by relpath.
func hashCorpus(corpusDir string) (string, error) {
	type entry struct {
		rel  string
		path string
	}
	var files []entry
	err := filepath.WalkDir(corpusDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Ext(p) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(corpusDir, p)
		if err != nil {
			return err
		}
		files = append(files, entry{rel: rel, path: p})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk corpus: %w", err)
	}
	slices.SortFunc(files, func(a, b entry) int {
		switch {
		case a.rel < b.rel:
			return -1
		case a.rel > b.rel:
			return 1
		default:
			return 0
		}
	})
	h := sha256.New()
	for _, e := range files {
		_, _ = h.Write([]byte(e.rel))
		_, _ = h.Write([]byte{0})
		data, err := os.ReadFile(e.path)
		if err != nil {
			return "", fmt.Errorf("read %q: %w", e.path, err)
		}
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
