package eval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

type fakePipeline struct {
	results map[string][]vectorstore.Result
	errs    map[string]error
	calls   int
	hook    func(call int) error
}

func (f *fakePipeline) Retrieve(ctx context.Context, query string, _ int) (string, error) {
	res, err := f.RetrieveChunks(ctx, query, 0)
	if err != nil {
		return "", err
	}
	parts := make([]string, len(res))
	for i, r := range res {
		parts[i] = r.Text
	}
	return strings.Join(parts, "\n---\n"), nil
}

func (f *fakePipeline) RetrieveChunks(_ context.Context, query string, _ int) ([]vectorstore.Result, error) {
	f.calls++
	if f.hook != nil {
		if err := f.hook(f.calls); err != nil {
			return nil, err
		}
	}
	if e, ok := f.errs[query]; ok {
		return nil, e
	}
	return f.results[query], nil
}

func TestRun_HappyAggregates(t *testing.T) {
	dataset := []GoldenQuery{
		{ID: "q1", Query: "alpha", ExpectedSources: []string{"a.md"}},
		{ID: "q2", Query: "beta", ExpectedSources: []string{"b.md"}},
		{ID: "q3", Query: "gamma", ExpectedSources: []string{"a.md", "b.md"}},
	}
	p := &fakePipeline{results: map[string][]vectorstore.Result{
		"alpha": {{Source: "a.md", Score: 0.9}, {Source: "c.md", Score: 0.5}},
		"beta":  {{Source: "c.md", Score: 0.7}, {Source: "b.md", Score: 0.6}},
		"gamma": {{Source: "a.md", Score: 0.8}, {Source: "b.md", Score: 0.7}},
	}}
	rep, err := Run(context.Background(), p, []string{"a.md", "b.md", "c.md"}, dataset, 5)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if rep.Partial {
		t.Fatalf("Partial should be false")
	}
	if len(rep.Queries) != 3 {
		t.Fatalf("expected 3 query results, got %d", len(rep.Queries))
	}
	if !rep.Queries[0].HitAtK || rep.Queries[0].ReciprocalRank != 1.0 {
		t.Fatalf("q1 wrong: %+v", rep.Queries[0])
	}
	if !rep.Queries[1].HitAtK || rep.Queries[1].ReciprocalRank != 0.5 {
		t.Fatalf("q2 wrong: %+v", rep.Queries[1])
	}
	// HitRate: 3/3, MRR: (1.0 + 0.5 + 1.0)/3 = 0.833...
	if rep.Summary.HitRate != 1.0 {
		t.Fatalf("HitRate: got %v want 1.0", rep.Summary.HitRate)
	}
	want := (1.0 + 0.5 + 1.0) / 3.0
	if rep.Summary.MRR < want-1e-9 || rep.Summary.MRR > want+1e-9 {
		t.Fatalf("MRR: got %v want %v", rep.Summary.MRR, want)
	}
	if rep.Summary.MaxSimilarity != 0.9 {
		t.Fatalf("MaxSimilarity: got %v want 0.9", rep.Summary.MaxSimilarity)
	}
}

func TestRun_PerQueryError(t *testing.T) {
	dataset := []GoldenQuery{
		{ID: "q1", Query: "ok", ExpectedSources: []string{"a.md"}},
		{ID: "q2", Query: "boom", ExpectedSources: []string{"a.md"}},
		{ID: "q3", Query: "ok2", ExpectedSources: []string{"a.md"}},
	}
	p := &fakePipeline{
		results: map[string][]vectorstore.Result{
			"ok":  {{Source: "a.md", Score: 0.9}},
			"ok2": {{Source: "a.md", Score: 0.8}},
		},
		errs: map[string]error{"boom": errors.New("retrieval blew up")},
	}
	rep, err := Run(context.Background(), p, []string{"a.md"}, dataset, 5)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if rep.Queries[1].Error == "" {
		t.Fatalf("expected q2 to record error")
	}
	if rep.Summary.HitRate != 1.0 {
		t.Fatalf("HitRate over successful queries should be 1.0, got %v", rep.Summary.HitRate)
	}
}

func TestRun_ContextCancelPartial(t *testing.T) {
	dataset := []GoldenQuery{
		{ID: "q1", Query: "first", ExpectedSources: []string{"a.md"}},
		{ID: "q2", Query: "second", ExpectedSources: []string{"a.md"}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &fakePipeline{
		results: map[string][]vectorstore.Result{
			"first":  {{Source: "a.md", Score: 0.9}},
			"second": {{Source: "a.md", Score: 0.8}},
		},
		hook: func(call int) error {
			if call == 1 {
				cancel()
				return context.Canceled
			}
			return nil
		},
	}
	rep, err := Run(ctx, p, []string{"a.md"}, dataset, 5)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if !rep.Partial {
		t.Fatalf("Partial should be true on cancel")
	}
	if rep.Summary != (Summary{}) {
		t.Fatalf("Summary should be zero on partial, got %+v", rep.Summary)
	}
}

func TestRun_RejectsUnknownExpectedSource(t *testing.T) {
	dataset := []GoldenQuery{
		{ID: "q1", Query: "x", ExpectedSources: []string{"missing.md"}},
	}
	_, err := Run(context.Background(), &fakePipeline{}, []string{"a.md"}, dataset, 5)
	if err == nil || !strings.Contains(err.Error(), "missing.md") {
		t.Fatalf("expected unknown-source error, got %v", err)
	}
}

func TestRun_RejectsZeroTopK(t *testing.T) {
	_, err := Run(context.Background(), &fakePipeline{}, nil, nil, 0)
	if err == nil {
		t.Fatalf("expected error for topK=0")
	}
}

func TestRun_RetrievedSourcesDedup(t *testing.T) {
	dataset := []GoldenQuery{
		{ID: "q1", Query: "x", ExpectedSources: []string{"a.md"}},
	}
	p := &fakePipeline{results: map[string][]vectorstore.Result{
		"x": {
			{Source: "a.md", ChunkIndex: 0, Score: 0.9},
			{Source: "a.md", ChunkIndex: 1, Score: 0.8},
			{Source: "b.md", ChunkIndex: 0, Score: 0.7},
		},
	}}
	rep, err := Run(context.Background(), p, []string{"a.md", "b.md"}, dataset, 5)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(rep.Queries[0].RetrievedSources) != 2 {
		t.Fatalf("expected 2 deduped sources, got %v", rep.Queries[0].RetrievedSources)
	}
	if len(rep.Queries[0].RetrievedSources) > 5 {
		t.Fatalf("retrieved sources exceed topK")
	}
}

type detEmbedder struct{}

func (detEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	// Deterministic 4-dim vector from first 4 bytes; lets retrieval order be predictable.
	v := []float32{0.1, 0.2, 0.3, 0.4}
	for i := 0; i < len(text) && i < 4; i++ {
		v[i] = float32(text[i]) / 255.0
	}
	return v, nil
}

func TestBuildHermeticPipeline(t *testing.T) {
	corpus := t.TempDir()
	for name, body := range map[string]string{
		"a.md": "alpha document about retrieval",
		"b.md": "beta document about generation",
	} {
		if err := os.WriteFile(filepath.Join(corpus, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	setup, err := BuildHermetic(context.Background(), detEmbedder{}, HermeticOptions{
		CorpusDir: corpus,
		ChunkSize: 64,
		WorkDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer setup.Cleanup()

	if len(setup.Sources) != 2 {
		t.Fatalf("expected 2 ingested sources, got %v", setup.Sources)
	}
	for _, src := range setup.Sources {
		if filepath.IsAbs(src) {
			t.Fatalf("source %q is absolute, expected corpus-relative", src)
		}
	}
	got, err := setup.Pipeline.RetrieveChunks(context.Background(), "alpha", 5)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected at least one retrieved chunk")
	}
}

func TestBuildHermetic_FreshCleanupRemovesTmpDir(t *testing.T) {
	corpus := t.TempDir()
	if err := os.WriteFile(filepath.Join(corpus, "a.md"), []byte("hi"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	workDir := t.TempDir()
	setup, err := BuildHermetic(context.Background(), detEmbedder{}, HermeticOptions{
		CorpusDir: corpus,
		ChunkSize: 64,
		WorkDir:   workDir,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	entries, _ := os.ReadDir(workDir)
	if len(entries) == 0 {
		t.Fatalf("expected a tmp-* dir under WorkDir before cleanup")
	}

	setup.Cleanup()

	entries, _ = os.ReadDir(workDir)
	if len(entries) != 0 {
		t.Fatalf("cleanup did not remove tmp dir, entries remain: %v", entries)
	}
}

func TestBuildHermetic_ReuseRoundTrip(t *testing.T) {
	corpus := t.TempDir()
	if err := os.WriteFile(filepath.Join(corpus, "a.md"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	workDir := t.TempDir()
	meta := HermeticMeta{EmbedModelDigest: "sha256:abc", CorpusHash: "sha256:c0", ChunkSize: 64}

	first, err := BuildHermetic(context.Background(), detEmbedder{}, HermeticOptions{
		CorpusDir: corpus, ChunkSize: 64, WorkDir: workDir, Reuse: true, Meta: meta,
	})
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	first.Cleanup()
	if _, err := os.Stat(filepath.Join(workDir, "run.db")); err != nil {
		t.Fatalf("cache db missing after first build: %v", err)
	}

	second, err := BuildHermetic(context.Background(), detEmbedder{}, HermeticOptions{
		CorpusDir: corpus, ChunkSize: 64, WorkDir: workDir, Reuse: true, Meta: meta,
	})
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	defer second.Cleanup()
	if len(second.Sources) != 1 || second.Sources[0] != "a.md" {
		t.Fatalf("reuse sources wrong: %v", second.Sources)
	}
}

func TestBuildHermetic_ReuseMetaMismatch(t *testing.T) {
	corpus := t.TempDir()
	if err := os.WriteFile(filepath.Join(corpus, "a.md"), []byte("hi"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	workDir := t.TempDir()
	first, err := BuildHermetic(context.Background(), detEmbedder{}, HermeticOptions{
		CorpusDir: corpus, ChunkSize: 64, WorkDir: workDir, Reuse: true,
		Meta: HermeticMeta{CorpusHash: "sha256:old"},
	})
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	first.Cleanup()

	_, err = BuildHermetic(context.Background(), detEmbedder{}, HermeticOptions{
		CorpusDir: corpus, ChunkSize: 64, WorkDir: workDir, Reuse: true,
		Meta: HermeticMeta{CorpusHash: "sha256:new"},
	})
	if err == nil {
		t.Fatalf("expected meta mismatch error")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale-cache error, got %v", err)
	}
	if !strings.Contains(err.Error(), "corpus_hash") {
		t.Fatalf("expected changed field name in error, got %v", err)
	}
}

func TestBuildHermetic_ReuseRequiresWorkDir(t *testing.T) {
	_, err := BuildHermetic(context.Background(), detEmbedder{}, HermeticOptions{
		CorpusDir: t.TempDir(), ChunkSize: 64, Reuse: true,
	})
	if err == nil {
		t.Fatalf("expected error when Reuse=true and WorkDir is empty")
	}
}

func TestBuildHermetic_FreshNoWorkDir(t *testing.T) {
	corpus := t.TempDir()
	if err := os.WriteFile(filepath.Join(corpus, "a.md"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	setup, err := BuildHermetic(context.Background(), detEmbedder{}, HermeticOptions{
		CorpusDir: corpus, ChunkSize: 64,
		// WorkDir intentionally omitted — should use OS temp dir, no project footprint
	})
	if err != nil {
		t.Fatalf("fresh build without WorkDir: %v", err)
	}
	setup.Cleanup()
}

func TestMedianInt64(t *testing.T) {
	cases := []struct {
		in   []int64
		want float64
	}{
		{[]int64{}, 0},
		{[]int64{42}, 42},
		{[]int64{1, 2, 3}, 2},
		{[]int64{4, 1, 3, 2}, 2.5},
		{[]int64{10, 20, 30, 40, 50, 60}, 35},
	}
	for _, tc := range cases {
		if got := medianInt64(tc.in); got != tc.want {
			t.Errorf("medianInt64(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
