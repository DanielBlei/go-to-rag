package eval

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleReport() *Report {
	return &Report{
		Config: &Config{
			EmbedModel:       "mxbai-embed-large:latest",
			EmbedModelDigest: "sha256:abc123",
			ChunkSize:        512,
			Overlap:          100,
			TopK:             5,
			CorpusPath:       "internal/eval/testdata/corpus",
			CorpusHash:       "sha256:c0c0",
			DatasetPath:      "internal/eval/testdata/golden.v1.json",
			DatasetHash:      "sha256:dada",
			RunStartedAt:     time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
		},
		Summary: Summary{
			HitRate:          0.85,
			MRR:              0.7125,
			Precision:        0.42,
			Recall:           0.78,
			MedianLatencyMS:  87,
			MinSimilarity:    0.31,
			MedianSimilarity: 0.62,
			MaxSimilarity:    0.91,
		},
		Queries: []QueryResult{
			{ID: "q1", Query: "alpha", ExpectedSources: []string{"a.md"},
				RetrievedSources: []string{"a.md"},
				HitAtK:           true, ReciprocalRank: 1.0, PrecisionAtK: 1.0, RecallAtK: 1.0,
				TopScore: 0.91, LatencyMS: 87},
			{ID: "q2", Query: "boom", ExpectedSources: []string{"b.md"}, Error: "retrieval failed"},
		},
	}
}

func TestWriteJSON_RoundTrip(t *testing.T) {
	rep := sampleReport()
	var buf bytes.Buffer
	if err := rep.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got jsonReport
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.NSamples != 2 || got.NFailed != 1 {
		t.Fatalf("counts wrong: nSamples=%d nFailed=%d", got.NSamples, got.NFailed)
	}
	if got.Results.HitRate != 0.85 {
		t.Fatalf("hit rate wrong: %v", got.Results.HitRate)
	}
	if got.Runtime.MedianLatencyMS != 87 {
		t.Fatalf("runtime median wrong: %v", got.Runtime.MedianLatencyMS)
	}
	if got.Similarity.Max != 0.91 {
		t.Fatalf("similarity max wrong: %v", got.Similarity.Max)
	}
	if got.HigherIsBetter["mrr"] != true {
		t.Fatalf("higher_is_better not serialized")
	}
	if _, ok := got.HigherIsBetter["top_score_max"]; ok {
		t.Fatalf("similarity stats must not appear in higher_is_better")
	}
	if got.Config == nil || got.Config.EmbedModelDigest != "sha256:abc123" {
		t.Fatalf("config not preserved: %+v", got.Config)
	}
	if len(got.Samples) != 2 || got.Samples[1].Error == "" {
		t.Fatalf("samples not preserved: %+v", got.Samples)
	}
}

func TestWriteJSON_Indented(t *testing.T) {
	var buf bytes.Buffer
	if err := sampleReport().WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !strings.Contains(buf.String(), "\n  \"results\"") {
		t.Fatalf("output is not indented: %s", buf.String())
	}
}

func TestWriteText_ContainsKeyMetrics(t *testing.T) {
	var buf bytes.Buffer
	if err := sampleReport().WriteText(&buf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"queries: 2 (failed: 1)",
		"top_k: 5",
		"embed: mxbai-embed-large:latest",
		"Hit@K",
		"0.8500",
		"MRR",
		"Precision@K",
		"Recall@K",
		"Latency (ms)",
		"Top score",
		"Per-query",
		"q1",
		"q2",
		"error: retrieval failed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestWriteText_EmptyReport(t *testing.T) {
	var buf bytes.Buffer
	rep := &Report{}
	if err := rep.WriteText(&buf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "queries: 0 (failed: 0)") {
		t.Fatalf("unexpected output for empty report: %s", out)
	}
	if strings.Contains(out, "Per-query") {
		t.Fatalf("empty report should not render per-query block")
	}
}

func TestWriteText_PartialMarker(t *testing.T) {
	rep := &Report{Partial: true}
	var buf bytes.Buffer
	if err := rep.WriteText(&buf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	if !strings.Contains(buf.String(), "partial: true") {
		t.Fatalf("partial marker not rendered: %s", buf.String())
	}
}

func TestWriteJSON_ZeroReport(t *testing.T) {
	var buf bytes.Buffer
	rep := &Report{}
	if err := rep.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got jsonReport
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.NSamples != 0 || got.NFailed != 0 {
		t.Fatalf("zero report should report n_samples=0 n_failed=0, got %+v", got)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errIO }

var errIO = stringErr("disk full")

type stringErr string

func (s stringErr) Error() string { return string(s) }

func TestWriteText_PropagatesWriteError(t *testing.T) {
	if err := sampleReport().WriteText(failingWriter{}); err == nil {
		t.Fatalf("expected error from failing writer")
	}
}
