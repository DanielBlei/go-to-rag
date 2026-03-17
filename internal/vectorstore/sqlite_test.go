package vectorstore

import (
	"context"
	"math"
	"testing"
)

func newMemStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func unitVec(dim, axis int) []float32 {
	v := make([]float32, dim)
	v[axis] = 1.0
	return v
}

func TestAddChunkAndCount(t *testing.T) {
	s := newMemStore(t)
	ctx := context.Background()

	count, err := s.CountChunks(ctx)
	if err != nil || count != 0 {
		t.Fatalf("expected 0 chunks, got %d err=%v", count, err)
	}

	if err := s.AddChunk(ctx, "doc.md", "hello world", unitVec(4, 0), 0); err != nil {
		t.Fatalf("AddChunk: %v", err)
	}

	count, err = s.CountChunks(ctx)
	if err != nil || count != 1 {
		t.Fatalf("expected 1 chunk, got %d err=%v", count, err)
	}
}

func TestHasSource(t *testing.T) {
	s := newMemStore(t)
	ctx := context.Background()

	ok, err := s.HasSource(ctx, "doc.md")
	if err != nil || ok {
		t.Fatalf("expected false, got %v err=%v", ok, err)
	}

	_ = s.AddChunk(ctx, "doc.md", "text", unitVec(4, 0), 0)

	ok, err = s.HasSource(ctx, "doc.md")
	if err != nil || !ok {
		t.Fatalf("expected true, got %v err=%v", ok, err)
	}

	ok, err = s.HasSource(ctx, "other.md")
	if err != nil || ok {
		t.Fatalf("expected false for other.md, got %v err=%v", ok, err)
	}
}

func TestDeleteSource(t *testing.T) {
	s := newMemStore(t)
	ctx := context.Background()

	_ = s.AddChunk(ctx, "doc.md", "chunk 0", unitVec(4, 0), 0)
	_ = s.AddChunk(ctx, "doc.md", "chunk 1", unitVec(4, 1), 1)
	_ = s.AddChunk(ctx, "other.md", "chunk 0", unitVec(4, 2), 0)

	if err := s.DeleteSource(ctx, "doc.md"); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}

	ok, _ := s.HasSource(ctx, "doc.md")
	if ok {
		t.Error("expected doc.md to be deleted")
	}

	ok, _ = s.HasSource(ctx, "other.md")
	if !ok {
		t.Error("other.md should not be affected")
	}

	count, _ := s.CountChunks(ctx)
	if count != 1 {
		t.Errorf("expected 1 chunk remaining, got %d", count)
	}
}

func TestSearchRankingAndScores(t *testing.T) {
	s := newMemStore(t)
	ctx := context.Background()

	// Three orthogonal unit vectors in 3D space.
	_ = s.AddChunk(ctx, "a.md", "chunk a", unitVec(3, 0), 0)
	_ = s.AddChunk(ctx, "b.md", "chunk b", unitVec(3, 1), 0)
	_ = s.AddChunk(ctx, "c.md", "chunk c", unitVec(3, 2), 0)

	// Query along axis 0 — "a" should be the top result.
	results, err := s.Search(ctx, unitVec(3, 0), 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Source != "a.md" {
		t.Errorf("expected a.md first, got %s", results[0].Source)
	}
	if math.Abs(results[0].Score-1.0) > 1e-6 {
		t.Errorf("expected score ~1.0 for identical vector, got %f", results[0].Score)
	}
	if math.Abs(results[1].Score) > 1e-6 || math.Abs(results[2].Score) > 1e-6 {
		t.Errorf("expected score ~0 for orthogonal vectors, got %f %f", results[1].Score, results[2].Score)
	}
}

func TestSearchLimit(t *testing.T) {
	s := newMemStore(t)
	ctx := context.Background()

	for i := range 5 {
		_ = s.AddChunk(ctx, "doc.md", "text", unitVec(3, 0), i)
	}

	results, err := s.Search(ctx, unitVec(3, 0), 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestSearchEmptyStore(t *testing.T) {
	s := newMemStore(t)
	ctx := context.Background()

	results, err := s.Search(ctx, unitVec(3, 0), 5)
	if err != nil {
		t.Fatalf("Search on empty store: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1.0},
		{"opposite", []float32{1, 0, 0}, []float32{-1, 0, 0}, -1.0},
		{"orthogonal", []float32{1, 0, 0}, []float32{0, 1, 0}, 0.0},
		{"zero vector", []float32{0, 0, 0}, []float32{1, 0, 0}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("cosineSimilarity = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestEmbeddingRoundTrip(t *testing.T) {
	original := []float32{1.5, -2.25, 0, 3.14159}
	decoded := decodeEmbedding(encodeEmbedding(original))
	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: got %d want %d", len(decoded), len(original))
	}
	for i := range original {
		if original[i] != decoded[i] {
			t.Errorf("index %d: got %v want %v", i, decoded[i], original[i])
		}
	}
}
