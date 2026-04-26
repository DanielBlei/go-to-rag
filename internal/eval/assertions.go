// Package eval provides a deterministic, LLM-free retrieval evaluation harness.
package eval

import "github.com/DanielBlei/go-to-rag/internal/vectorstore"

// sourceSet returns the unique sources in the first k results.
// k is capped at len(retrieved); non-positive k returns an empty set.
func sourceSet(retrieved []vectorstore.Result, k int) map[string]struct{} {
	if k <= 0 {
		return map[string]struct{}{}
	}
	if k > len(retrieved) {
		k = len(retrieved)
	}
	set := make(map[string]struct{}, k)
	for i := 0; i < k; i++ {
		set[retrieved[i].Source] = struct{}{}
	}
	return set
}

// expectedSet builds a set from the expected sources slice.
func expectedSet(expected []string) map[string]struct{} {
	set := make(map[string]struct{}, len(expected))
	for _, s := range expected {
		set[s] = struct{}{}
	}
	return set
}

// HitAtK reports whether any expected source appears in the top-k retrieved results.
func HitAtK(retrieved []vectorstore.Result, expected []string, k int) bool {
	if len(expected) == 0 {
		return false
	}
	got := sourceSet(retrieved, k)
	for _, s := range expected {
		if _, ok := got[s]; ok {
			return true
		}
	}
	return false
}

// ReciprocalRank scores by rank of first match (1/Rank)
// Sources are deduplicated so multiple chunks from the same doc count as one position.
func ReciprocalRank(retrieved []vectorstore.Result, expected []string) float64 {
	if len(expected) == 0 || len(retrieved) == 0 {
		return 0
	}
	want := expectedSet(expected)
	seen := make(map[string]struct{}, len(retrieved))
	rank := 0
	for _, r := range retrieved {
		if _, dup := seen[r.Source]; dup {
			continue
		}
		seen[r.Source] = struct{}{}
		rank++
		if _, ok := want[r.Source]; ok {
			return 1.0 / float64(rank)
		}
	}
	return 0
}

// PrecisionAtK is |sourceSet(retrieved, k) ∩ expected| / |sourceSet(retrieved, k)|.
// Returns 0 when no sources are retrieved.
//
// Note: this is source-level precision, not chunk-level P@K.
// Multiple chunks from the same source collapse into one before scoring.
func PrecisionAtK(retrieved []vectorstore.Result, expected []string, k int) float64 {
	got := sourceSet(retrieved, k)
	if len(got) == 0 {
		return 0
	}
	want := expectedSet(expected)
	hits := 0
	for s := range got {
		if _, ok := want[s]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(got))
}

// RecallAtK is |sourceSet(retrieved, k) ∩ expected| / |expected|.
// Returns 0 when expected is empty.
//
// Note: this is source-level recall, not chunk-level R@K.
// Multiple chunks from the same source collapse into one before scoring.
// When k < |expected|, recall is bounded above by k/|expected|. Pick k with
// that ceiling in mind for multi-source queries, otherwise a perfect retriever
// will still score below 1.0 on the recall axis.
func RecallAtK(retrieved []vectorstore.Result, expected []string, k int) float64 {
	if len(expected) == 0 {
		return 0
	}
	got := sourceSet(retrieved, k)
	want := expectedSet(expected)
	hits := 0
	for s := range got {
		if _, ok := want[s]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(want))
}
