package eval

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/DanielBlei/go-to-rag/internal/ingest"
	"github.com/DanielBlei/go-to-rag/internal/rag"
	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

// QueryResult is the per-query outcome of a single retrieval evaluation.
type QueryResult struct {
	ID               string   `json:"id"`
	Query            string   `json:"query"`
	ExpectedSources  []string `json:"expected_sources"`
	RetrievedSources []string `json:"retrieved_sources"`
	HitAtK           bool     `json:"hit_at_k"`
	ReciprocalRank   float64  `json:"reciprocal_rank"`
	PrecisionAtK     float64  `json:"precision_at_k"`
	RecallAtK        float64  `json:"recall_at_k"`
	// TopScore is retrieved[0].Score when retrieved is non-empty, else 0.
	// It is not the maximum across the top-K, despite what the name might
	// suggest at first read.
	TopScore  float64 `json:"top_score"`
	LatencyMS int64   `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`

	// Judge is reserved for a future LLM judge phase. Always nil in v1.
	Judge *JudgeResult `json:"judge,omitempty"`
}

// JudgeResult is reserved for a future LLM judge.
type JudgeResult struct {
	// intentionally empty; placeholder for schema stability
}

// Summary holds aggregate metrics computed over the per-query results that
// did not error. When Partial is true on the parent Report, Summary is zero.
type Summary struct {
	HitRate          float64 `json:"hit_rate"`
	MRR              float64 `json:"mrr"`
	Precision        float64 `json:"precision"`
	Recall           float64 `json:"recall"`
	MedianLatencyMS  float64 `json:"median_latency_ms"`
	MinSimilarity    float64 `json:"min_similarity"`
	MedianSimilarity float64 `json:"median_similarity"`
	MaxSimilarity    float64 `json:"max_similarity"`
}

// Embedder is the minimal interface needed to embed query text. Same shape
// as rag.Embedder; redeclared so callers can pass any compatible client.
type Embedder = rag.Embedder

// HermeticSetup bundles the result of BuildHermeticPipeline for clarity.
type HermeticSetup struct {
	Pipeline  rag.Pipeline
	Sources   []string
	Cleanup   func()
}

// BuildHermeticPipeline ingests every *.md file under corpusDir into a fresh
// SQLite vector store and returns a HermeticSetup backed by that store.
// The list of ingested source identifiers is corpus-relative paths.
//
// Callers must call Cleanup even on error from Run.
func BuildHermeticPipeline(
	ctx context.Context,
	embedder Embedder,
	corpusDir string,
	chunkSize, overlap int,
) (*HermeticSetup, error) {
	tmpDir, err := os.MkdirTemp("", "go-to-rag-eval-*")
	if err != nil {
		return nil, fmt.Errorf("eval: mkdir temp: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	dbPath := filepath.Join(tmpDir, "eval.db")
	store, err := vectorstore.NewSQLite(dbPath)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("eval: open vector store: %w", err)
	}
	combined := func() {
		_ = store.Close()
		_ = os.RemoveAll(tmpDir)
	}

	sourcePath := func(p string) (string, error) {
		rel, err := filepath.Rel(corpusDir, p)
		if err != nil {
			return "", err
		}
		return rel, nil
	}

	sources, _, err := ingest.Run(ctx, store, embedder, corpusDir, ingest.Options{
		ChunkSize:    chunkSize,
		Overlap:      overlap,
		Glob:         "*.md",
		SkipExisting: false,
		SourcePath:   sourcePath,
	})
	if err != nil {
		combined()
		return nil, fmt.Errorf("eval: ingest corpus: %w", err)
	}

	return &HermeticSetup{
		Pipeline: rag.NewPipeline(embedder, store),
		Sources:  sources,
		Cleanup:  combined,
	}, nil
}

// Run executes every query in dataset against pipeline and returns a Report.
// On context cancellation, the report's Partial flag is set, Summary is zero,
// and Run returns ctx.Err() alongside the partial report.
//
// Run validates that every expected source in dataset appears in ingested
// before running any queries; an unknown source fails loud with an error.
func Run(
	ctx context.Context,
	p rag.Pipeline,
	ingested []string,
	dataset []GoldenQuery,
	topK int,
) (*Report, error) {
	if topK <= 0 {
		return nil, fmt.Errorf("eval: topK must be > 0, got %d", topK)
	}
	known := make(map[string]struct{}, len(ingested))
	for _, s := range ingested {
		known[s] = struct{}{}
	}
	for _, q := range dataset {
		for _, src := range q.ExpectedSources {
			if _, ok := known[src]; !ok {
				return nil, fmt.Errorf("eval: query %s expects unknown source %q (not in ingested corpus)", q.ID, src)
			}
		}
	}

	results := make([]QueryResult, 0, len(dataset))
	for _, q := range dataset {
		if err := ctx.Err(); err != nil {
			return &Report{Queries: results, Partial: true}, err
		}
		qr := QueryResult{
			ID:              q.ID,
			Query:           q.Query,
			ExpectedSources: q.ExpectedSources,
		}
		start := time.Now()
		retrieved, err := p.RetrieveChunks(ctx, q.Query, topK)
		qr.LatencyMS = time.Since(start).Milliseconds()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				results = append(results, qr)
				return &Report{Queries: results, Partial: true}, err
			}
			qr.Error = err.Error()
			results = append(results, qr)
			continue
		}
		qr.RetrievedSources = dedupSources(retrieved)
		qr.HitAtK = HitAtK(retrieved, q.ExpectedSources, topK)
		qr.ReciprocalRank = ReciprocalRank(retrieved, q.ExpectedSources)
		qr.PrecisionAtK = PrecisionAtK(retrieved, q.ExpectedSources, topK)
		qr.RecallAtK = RecallAtK(retrieved, q.ExpectedSources, topK)
		if len(retrieved) > 0 {
			qr.TopScore = retrieved[0].Score
		}
		results = append(results, qr)
	}

	rep := &Report{Queries: results, Summary: aggregate(results)}
	return rep, nil
}

// Report is the full output of a single evaluation run.
type Report struct {
	Config  *Config       `json:"config,omitempty"`
	Summary Summary       `json:"summary"`
	Queries []QueryResult `json:"queries"`
	Partial bool          `json:"partial,omitempty"`
}

// Config records the inputs to a run for reproducibility.
// Populated by the CLI in M6; runner does not set it.
type Config struct {
	EmbedModel       string    `json:"embed_model,omitempty"`
	EmbedModelDigest string    `json:"embed_model_digest,omitempty"`
	ChunkSize        int       `json:"chunk_size,omitempty"`
	Overlap          int       `json:"overlap,omitempty"`
	TopK             int       `json:"top_k,omitempty"`
	CorpusPath       string    `json:"corpus_path,omitempty"`
	CorpusHash       string    `json:"corpus_hash,omitempty"`
	DatasetPath      string    `json:"dataset_path,omitempty"`
	DatasetHash      string    `json:"dataset_hash,omitempty"`
	RunStartedAt     time.Time `json:"run_started_at,omitzero"`
}

func dedupSources(retrieved []vectorstore.Result) []string {
	seen := make(map[string]struct{}, len(retrieved))
	out := make([]string, 0, len(retrieved))
	for _, r := range retrieved {
		if _, ok := seen[r.Source]; ok {
			continue
		}
		seen[r.Source] = struct{}{}
		out = append(out, r.Source)
	}
	return out
}

func aggregate(results []QueryResult) Summary {
	var s Summary
	successful := 0
	latencies := make([]int64, 0, len(results))
	scores := make([]float64, 0, len(results))
	for _, q := range results {
		if q.Error != "" {
			continue
		}
		successful++
		if q.HitAtK {
			s.HitRate++
		}
		s.MRR += q.ReciprocalRank
		s.Precision += q.PrecisionAtK
		s.Recall += q.RecallAtK
		latencies = append(latencies, q.LatencyMS)
		scores = append(scores, q.TopScore)
	}
	if successful == 0 {
		return Summary{}
	}
	n := float64(successful)
	s.HitRate /= n
	s.MRR /= n
	s.Precision /= n
	s.Recall /= n
	s.MedianLatencyMS = medianInt64(latencies)
	s.MinSimilarity, s.MedianSimilarity, s.MaxSimilarity = minMedianMax(scores)
	return s
}

func medianInt64(xs []int64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := make([]int64, len(xs))
	copy(sorted, xs)
	slices.Sort(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return float64(sorted[n/2])
	}
	return float64(sorted[n/2-1]+sorted[n/2]) / 2
}

func minMedianMax(xs []float64) (mn, med, mx float64) {
	if len(xs) == 0 {
		return 0, 0, 0
	}
	sorted := make([]float64, len(xs))
	copy(sorted, xs)
	slices.Sort(sorted)
	mn = sorted[0]
	mx = sorted[len(sorted)-1]
	n := len(sorted)
	if n%2 == 1 {
		med = sorted[n/2]
	} else {
		med = (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return mn, med, mx
}
