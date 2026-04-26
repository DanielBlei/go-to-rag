package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"text/tabwriter"
)

// HigherIsBetter maps each retrieval-quality metric to whether a larger value
// is better. Borrowed from lm-evaluation-harness convention so downstream
// tooling can orient on metrics it has not seen before.
//
// Only quality metrics belong here. Latency and similarity statistics are
// diagnostic, not evaluative: a higher similarity floor reflects an easier
// dataset, not a better system. Putting them in this map would mislead any
// tool that auto-orients on it.
var HigherIsBetter = map[string]bool{
	"hit_rate":  true,
	"mrr":       true,
	"precision": true,
	"recall":    true,
}

// resultsBlock holds retrieval-quality metrics. Auto-orient via HigherIsBetter.
type resultsBlock struct {
	HitRate   float64 `json:"hit_rate"`
	MRR       float64 `json:"mrr"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
}

// runtimeBlock holds runtime characteristics — diagnostic, not evaluative.
type runtimeBlock struct {
	MedianLatencyMS float64 `json:"median_latency_ms"`
	P95LatencyMS    float64 `json:"p95_latency_ms"`
}

// similarityBlock holds rank-1 similarity statistics across queries —
// diagnostic, reflects dataset difficulty more than system quality.
type similarityBlock struct {
	Min    float64 `json:"min"`
	Median float64 `json:"median"`
	Max    float64 `json:"max"`
}

// jsonReport is the on-disk shape. Field order mirrors lm-evaluation-harness:
// quality results separated from runtime, configs, and per-sample logs.
type jsonReport struct {
	Results          resultsBlock           `json:"results"`
	PerType          map[string]TypeSummary `json:"per_type,omitempty"`
	RankDistribution RankHistogram          `json:"rank_distribution"`
	Runtime          runtimeBlock           `json:"runtime"`
	Similarity       similarityBlock        `json:"similarity"`
	HigherIsBetter   map[string]bool        `json:"higher_is_better"`
	NSamples         int                    `json:"n_samples"`
	NFailed          int                    `json:"n_failed"`
	Partial          bool                   `json:"partial,omitempty"`
	Config           *Config                `json:"config,omitempty"`
	Samples          []QueryResult          `json:"samples"`
}

func (r *Report) toJSON() jsonReport {
	failed := 0
	for _, q := range r.Queries {
		if q.Error != "" {
			failed++
		}
	}
	return jsonReport{
		Results: resultsBlock{
			HitRate:   r.Summary.HitRate,
			MRR:       r.Summary.MRR,
			Precision: r.Summary.Precision,
			Recall:    r.Summary.Recall,
		},
		PerType:          r.Summary.PerType,
		RankDistribution: r.Summary.RankDistribution,
		Runtime: runtimeBlock{
			MedianLatencyMS: r.Summary.MedianLatencyMS,
			P95LatencyMS:    r.Summary.P95LatencyMS,
		},
		Similarity: similarityBlock{
			Min:    r.Summary.MinSimilarity,
			Median: r.Summary.MedianSimilarity,
			Max:    r.Summary.MaxSimilarity,
		},
		HigherIsBetter: HigherIsBetter,
		NSamples:       len(r.Queries),
		NFailed:        failed,
		Partial:        r.Partial,
		Config:         r.Config,
		Samples:        r.Queries,
	}
}

func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r.toJSON()); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

// WriteText writes a human-scannable report. Layout is intentionally flat.
func (r *Report) WriteText(w io.Writer) error {
	failed := 0
	for _, q := range r.Queries {
		if q.Error != "" {
			failed++
		}
	}
	bw := newErrWriter(w)
	bw.printf("Eval Report\n")
	bw.printf("  queries: %d (failed: %d)\n", len(r.Queries), failed)
	if r.Config != nil {
		if r.Config.TopK > 0 {
			bw.printf("  top_k: %d\n", r.Config.TopK)
		}
		if r.Config.EmbedModel != "" {
			bw.printf("  embed: %s\n", r.Config.EmbedModel)
		}
		if r.Config.EmbedModelDigest != "" {
			bw.printf("  embed_digest: %s\n", r.Config.EmbedModelDigest)
		}
		if r.Config.CorpusHash != "" {
			bw.printf("  corpus_hash: %s\n", r.Config.CorpusHash)
		}
		if r.Config.DatasetHash != "" {
			bw.printf("  dataset_hash: %s\n", r.Config.DatasetHash)
		}
	}
	if r.Partial {
		bw.printf("  partial: true (run was canceled)\n")
	}

	bw.printf("\nAggregate\n")
	bw.printf("  Hit@K        %.4f\n", r.Summary.HitRate)
	bw.printf("  MRR          %.4f\n", r.Summary.MRR)
	bw.printf("  Precision@K  %.4f\n", r.Summary.Precision)
	bw.printf("  Recall@K     %.4f\n", r.Summary.Recall)

	if len(r.Summary.PerType) > 0 {
		bw.printf("\nBy query type\n")
		tw := tabwriter.NewWriter(bw, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  type\tn\thit\tmrr\tp@k\tr@k")
		for _, t := range sortedTypes(r.Summary.PerType) {
			ts := r.Summary.PerType[t]
			_, _ = fmt.Fprintf(tw, "  %s\t%d\t%.4f\t%.4f\t%.4f\t%.4f\n",
				t, ts.N, ts.HitRate, ts.MRR, ts.Precision, ts.Recall)
		}
		if err := tw.Flush(); err != nil {
			return fmt.Errorf("flush per-type block: %w", err)
		}
	}

	rd := r.Summary.RankDistribution
	bw.printf("\nRank of first hit\n")
	bw.printf("  rank=1    %d\n", rd.Rank1)
	bw.printf("  rank=2-3  %d\n", rd.Rank2_3)
	bw.printf("  rank=4-K  %d\n", rd.Rank4_K)
	bw.printf("  miss      %d\n", rd.Miss)

	bw.printf("\nLatency (ms)\n")
	bw.printf("  median  %.1f\n", r.Summary.MedianLatencyMS)
	bw.printf("  p95     %.1f\n", r.Summary.P95LatencyMS)

	bw.printf("\nTop score (rank-1 similarity)\n")
	bw.printf("  min     %.4f\n", r.Summary.MinSimilarity)
	bw.printf("  median  %.4f\n", r.Summary.MedianSimilarity)
	bw.printf("  max     %.4f\n", r.Summary.MaxSimilarity)

	if len(r.Queries) > 0 {
		bw.printf("\nPer-query\n")
		tw := tabwriter.NewWriter(bw, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  id\ttype\thit\trank\trr\tp@k\tr@k\ttop\tms\tnote")
		for _, q := range r.Queries {
			note := ""
			if q.Error != "" {
				note = "error: " + q.Error
			}
			rank := "-"
			if q.Error == "" {
				if k := rankFromRR(q.ReciprocalRank); k > 0 {
					rank = fmt.Sprintf("%d", k)
				}
			}
			typeStr := q.Type
			if typeStr == "" {
				typeStr = "-"
			}
			_, _ = fmt.Fprintf(
				tw,
				"  %s\t%s\t%v\t%s\t%.2f\t%.2f\t%.2f\t%.3f\t%d\t%s\n",
				q.ID,
				typeStr,
				q.HitAtK,
				rank,
				q.ReciprocalRank,
				q.PrecisionAtK,
				q.RecallAtK,
				q.TopScore,
				q.LatencyMS,
				note,
			)
		}
		if err := tw.Flush(); err != nil {
			return fmt.Errorf("flush per-query block: %w", err)
		}
	}
	return bw.err
}

// sortedTypes returns the keys of m in a stable order: known types first
// (direct, multi-doc, adversarial), then any others alphabetically.
func sortedTypes(m map[string]TypeSummary) []string {
	order := []string{TypeDirect, TypeMultiDoc, TypeAdversarial}
	out := make([]string, 0, len(m))
	seen := make(map[string]struct{}, len(m))
	for _, t := range order {
		if _, ok := m[t]; ok {
			out = append(out, t)
			seen[t] = struct{}{}
		}
	}
	var rest []string
	for t := range m {
		if _, ok := seen[t]; ok {
			continue
		}
		rest = append(rest, t)
	}
	slices.Sort(rest)
	out = append(out, rest...)
	return out
}

type errWriter struct {
	w   io.Writer
	err error
}

func newErrWriter(w io.Writer) *errWriter { return &errWriter{w: w} }

func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	if err != nil {
		e.err = err
	}
	return n, err
}

func (e *errWriter) printf(format string, args ...any) {
	if e.err != nil {
		return
	}
	if _, err := fmt.Fprintf(e.w, format, args...); err != nil {
		e.err = err
	}
}
