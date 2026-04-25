// Package eval provides a deterministic, LLM-free retrieval evaluation harness.
//
// Metric semantics: source-granularity, binary relevance. A retrieved chunk is
// relevant iff its Source matches one of the query's expected sources by exact
// string equality. Multiple chunks from the same source collapse to one match
// (deduplicated by Source).
package eval

import "github.com/DanielBlei/go-to-rag/internal/vectorstore"

// sourceSet returns the unique sources in the first k results.
// k is clamped to [0, len(retrieved)].
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

// HitAtK reports whether any expected source appears in the top-k retrieved
// results.
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

// ReciprocalRank returns 1/rank of the first retrieved result whose source is
// in expected (1-indexed, source-deduplicated). Returns 0 if no expected source
// is retrieved.
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