package eval

import (
	"encoding/json"
	"fmt"
	"io"
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
	Results        resultsBlock    `json:"results"`
	Runtime        runtimeBlock    `json:"runtime"`
	Similarity     similarityBlock `json:"similarity"`
	HigherIsBetter map[string]bool `json:"higher_is_better"`
	NSamples       int             `json:"n_samples"`
	NFailed        int             `json:"n_failed"`
	Partial        bool            `json:"partial,omitempty"`
	Config         *Config         `json:"config,omitempty"`
	Samples        []QueryResult   `json:"samples"`
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
		Runtime: runtimeBlock{MedianLatencyMS: r.Summary.MedianLatencyMS},
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

// WriteJSON writes the report as a stable, indented JSON document.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r.toJSON()); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

// WriteText writes a human-scannable report. Layout is intentionally flat;
// the JSON output is the source of truth for downstream tooling.
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

	bw.printf("\nLatency (ms)\n")
	bw.printf("  median  %.1f\n", r.Summary.MedianLatencyMS)

	bw.printf("\nTop score (rank-1 similarity)\n")
	bw.printf("  min     %.4f\n", r.Summary.MinSimilarity)
	bw.printf("  median  %.4f\n", r.Summary.MedianSimilarity)
	bw.printf("  max     %.4f\n", r.Summary.MaxSimilarity)

	if len(r.Queries) > 0 {
		bw.printf("\nPer-query\n")
		tw := tabwriter.NewWriter(bw, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  id\thit\trr\tp@k\tr@k\ttop\tms\tnote")
		for _, q := range r.Queries {
			note := ""
			if q.Error != "" {
				note = "error: " + q.Error
			}
			_, _ = fmt.Fprintf(tw, "  %s\t%v\t%.2f\t%.2f\t%.2f\t%.3f\t%d\t%s\n",
				q.ID, q.HitAtK, q.ReciprocalRank, q.PrecisionAtK, q.RecallAtK, q.TopScore, q.LatencyMS, note)
		}
		if err := tw.Flush(); err != nil {
			return fmt.Errorf("flush per-query block: %w", err)
		}
	}
	return bw.err
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
