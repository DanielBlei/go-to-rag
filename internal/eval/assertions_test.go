package eval

import (
	"math"
	"testing"

	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

func r(sources ...string) []vectorstore.Result {
	out := make([]vectorstore.Result, len(sources))
	for i, s := range sources {
		out[i] = vectorstore.Result{Source: s, Score: 1.0 - float64(i)*0.1}
	}
	return out
}

func almost(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestHitAtK(t *testing.T) {
	cases := []struct {
		name      string
		retrieved []vectorstore.Result
		expected  []string
		k         int
		want      bool
	}{
		{"hit at 1", r("a.md", "b.md"), []string{"a.md"}, 1, true},
		{"hit at k", r("b.md", "c.md", "a.md"), []string{"a.md"}, 3, true},
		{"miss within k", r("b.md", "c.md", "a.md"), []string{"a.md"}, 2, false},
		{"empty retrieved", nil, []string{"a.md"}, 5, false},
		{"empty expected", r("a.md"), nil, 5, false},
		{"k zero", r("a.md"), []string{"a.md"}, 0, false},
		{"k negative", r("a.md"), []string{"a.md"}, -1, false},
		{"k larger than retrieved", r("a.md"), []string{"a.md"}, 100, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HitAtK(tc.retrieved, tc.expected, tc.k); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReciprocalRank(t *testing.T) {
	cases := []struct {
		name      string
		retrieved []vectorstore.Result
		expected  []string
		want      float64
	}{
		{"first", r("a.md", "b.md"), []string{"a.md"}, 1.0},
		{"second", r("b.md", "a.md"), []string{"a.md"}, 0.5},
		{"third with dedup", r("b.md", "b.md", "a.md"), []string{"a.md"}, 0.5},
		{"none", r("b.md", "c.md"), []string{"a.md"}, 0.0},
		{"empty retrieved", nil, []string{"a.md"}, 0.0},
		{"empty expected", r("a.md"), nil, 0.0},
		{"dup expected source uses first", r("a.md", "b.md", "a.md"), []string{"a.md"}, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			almost(t, ReciprocalRank(tc.retrieved, tc.expected), tc.want)
		})
	}
}

func TestPrecisionAtK(t *testing.T) {
	cases := []struct {
		name      string
		retrieved []vectorstore.Result
		expected  []string
		k         int
		want      float64
	}{
		{"perfect", r("a.md", "b.md"), []string{"a.md", "b.md"}, 2, 1.0},
		{"half", r("a.md", "c.md"), []string{"a.md"}, 2, 0.5},
		{"none", r("c.md", "d.md"), []string{"a.md"}, 2, 0.0},
		{"dedup at source", r("a.md", "a.md", "b.md"), []string{"a.md"}, 3, 0.5},
		{"empty retrieved", nil, []string{"a.md"}, 2, 0.0},
		{"k zero", r("a.md"), []string{"a.md"}, 0, 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			almost(t, PrecisionAtK(tc.retrieved, tc.expected, tc.k), tc.want)
		})
	}
}

func TestRecallAtK(t *testing.T) {
	cases := []struct {
		name      string
		retrieved []vectorstore.Result
		expected  []string
		k         int
		want      float64
	}{
		{"perfect", r("a.md", "b.md"), []string{"a.md", "b.md"}, 2, 1.0},
		{"half of expected", r("a.md", "c.md"), []string{"a.md", "b.md"}, 2, 0.5},
		{"none", r("c.md"), []string{"a.md", "b.md"}, 2, 0.0},
		{"empty expected", r("a.md"), nil, 2, 0.0},
		{"empty retrieved", nil, []string{"a.md"}, 2, 0.0},
		{"k clips before second expected", r("a.md", "c.md", "b.md"), []string{"a.md", "b.md"}, 2, 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			almost(t, RecallAtK(tc.retrieved, tc.expected, tc.k), tc.want)
		})
	}
}

func TestSourceSetClamp(t *testing.T) {
	got := sourceSet(r("a.md", "b.md"), 10)
	if len(got) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(got))
	}
	if _, ok := got["a.md"]; !ok {
		t.Fatalf("missing a.md")
	}
	if _, ok := got["b.md"]; !ok {
		t.Fatalf("missing b.md")
	}
}
