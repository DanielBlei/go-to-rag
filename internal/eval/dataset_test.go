package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGolden(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "golden.v1.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestLoadGolden_OK(t *testing.T) {
	content := `[
  {"id": "q001", "query": "what is a pod", "expected_sources": ["pods.md"]},
  {"id": "q002", "query": "crd vs operator", "expected_sources": ["crds.md", "operators.md"]},
  {"id": "q003", "query": "olm", "expected_sources": ["olm.md"]}
]`
	got, err := LoadGolden(writeGolden(t, content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 records, got %d", len(got))
	}
	if got[1].ID != "q002" || len(got[1].ExpectedSources) != 2 {
		t.Fatalf("record 2 wrong: %+v", got[1])
	}
}

func TestLoadGolden_Errors(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			"malformed json",
			`[{"id":"q001","query":"x","expected_sources":["a.md"]}, not json]`,
			"record 2: parse",
		},
		{
			"not an array",
			`{"id":"q1","query":"x","expected_sources":["a.md"]}`,
			"expected JSON array",
		},
		{
			"duplicate id",
			`[{"id":"q1","query":"x","expected_sources":["a.md"]},
			  {"id":"q1","query":"y","expected_sources":["b.md"]}]`,
			"duplicate id",
		},
		{
			"empty id",
			`[{"id":"","query":"x","expected_sources":["a.md"]}]`,
			"empty id",
		},
		{
			"empty query",
			`[{"id":"q1","query":"","expected_sources":["a.md"]}]`,
			"empty query",
		},
		{
			"empty expected_sources",
			`[{"id":"q1","query":"x","expected_sources":[]}]`,
			"empty expected_sources",
		},
		{
			"blank source string",
			`[{"id":"q1","query":"x","expected_sources":["  "]}]`,
			"is blank",
		},
		{
			"duplicate expected source",
			`[{"id":"q1","query":"x","expected_sources":["a.md","a.md"]}]`,
			"duplicates",
		},
		{
			"unknown field rejected",
			`[{"id":"q1","query":"x","expected_sources":["a.md"],"extra":true}]`,
			"parse",
		},
		{
			"empty array",
			`[]`,
			"no records",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadGolden(writeGolden(t, tc.content))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestLoadGolden_StripsBOM(t *testing.T) {
	content := "\xEF\xBB\xBF" + `[{"id":"q1","query":"x","expected_sources":["a.md"]}]`
	got, err := LoadGolden(writeGolden(t, content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "q1" {
		t.Fatalf("BOM not stripped, got %+v", got)
	}
}

func TestLoadGolden_FileNotFound(t *testing.T) {
	_, err := LoadGolden("/nonexistent/path/golden.v1.json")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "golden open") {
		t.Fatalf("expected wrap with 'golden open', got %v", err)
	}
}

func TestLoadGolden_RealGoldenFile(t *testing.T) {
	got, err := LoadGolden("testdata/golden.v1.json")
	if err != nil {
		t.Fatalf("real golden file failed to load: %v", err)
	}
	if len(got) != 20 {
		t.Fatalf("expected 20 queries, got %d", len(got))
	}
	corpusFiles := map[string]bool{
		"kubernetes_pods.md":          true,
		"kubernetes_operators.md":     true,
		"kubernetes_crds.md":          true,
		"olm_architecture.md":         true,
		"openshift_routes.md":         true,
		"kubebuilder_introduction.md": true,
	}
	for _, q := range got {
		for _, src := range q.ExpectedSources {
			if !corpusFiles[src] {
				t.Errorf("%s: expected_source %q is not in the frozen corpus", q.ID, src)
			}
		}
	}
}

func TestLoadGolden_SmokeFixture(t *testing.T) {
	got, err := LoadGolden("testdata/smoke/golden.v1.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 smoke records, got %d", len(got))
	}
	if got[0].ID != "s001" || got[2].ID != "s003" {
		t.Fatalf("ids wrong: %+v", got)
	}
	if len(got[2].ExpectedSources) != 2 {
		t.Fatalf("s003 should have 2 expected sources, got %v", got[2].ExpectedSources)
	}
}