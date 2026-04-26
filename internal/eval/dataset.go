package eval

// Golden dataset schema (golden.v1.json): a JSON array of records.
//
//	[
//	  {"id": "q001", "query": "...", "expected_sources": ["foo.md", ...]},
//	  ...
//	]
//
// Future v2 may add graded relevance (per-source scores) without breaking v1 readers.
// Loaders should reject unknown top-level fields only at major version bumps.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// utf8BOM is the UTF-8 byte order mark.
// Some editors (Excel, Notepad) add it when saving UTF-8 files
// LoadGolden strips it before JSON parsing.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Query types used to slice the aggregate report. Optional on each record:
// untyped queries still count toward the overall aggregate, but do not
// contribute to any per-type rollup.
const (
	TypeDirect      = "direct"
	TypeMultiDoc    = "multi-doc"
	TypeAdversarial = "adversarial"
)

var validTypes = map[string]struct{}{
	TypeDirect:      {},
	TypeMultiDoc:    {},
	TypeAdversarial: {},
}

// GoldenQuery is a single record in a golden.v1.json file.
type GoldenQuery struct {
	ID              string   `json:"id"`
	Query           string   `json:"query"`
	ExpectedSources []string `json:"expected_sources"`
	// Type is optional. When set, must be one of TypeDirect, TypeMultiDoc,
	// TypeAdversarial. Empty string is allowed and means untyped.
	Type string `json:"type,omitempty"`
}

// LoadGolden reads a golden.v1.json file from path and returns the parsed queries.
// Unknown fields, duplicate IDs, and records with empty required fields cause an error annotated with the record index.
func LoadGolden(path string) ([]GoldenQuery, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("golden open: %w", err)
	}
	defer func() { _ = f.Close() }()

	br := bufio.NewReader(f)
	if bom, _ := br.Peek(3); bytes.Equal(bom, utf8BOM) {
		_, _ = br.Discard(3)
	}

	dec := json.NewDecoder(br)
	dec.DisallowUnknownFields()

	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("golden %s: read opening token: %w", path, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, fmt.Errorf("golden %s: expected JSON array, got %v", path, tok)
	}

	var (
		queries []GoldenQuery
		seen    = map[string]int{}
		idx     int
	)
	for dec.More() {
		idx++
		var q GoldenQuery
		if err := dec.Decode(&q); err != nil {
			return nil, fmt.Errorf("golden %s record %d: parse: %w", path, idx, err)
		}
		if q.ID == "" {
			return nil, fmt.Errorf("golden %s record %d: empty id", path, idx)
		}
		if q.Query == "" {
			return nil, fmt.Errorf("golden %s record %d: empty query (id=%s)", path, idx, q.ID)
		}
		if len(q.ExpectedSources) == 0 {
			return nil, fmt.Errorf("golden %s record %d: empty expected_sources (id=%s)", path, idx, q.ID)
		}
		seenSrc := make(map[string]struct{}, len(q.ExpectedSources))
		for i, s := range q.ExpectedSources {
			if strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("golden %s record %d: expected_sources[%d] is blank (id=%s)", path, idx, i, q.ID)
			}
			if _, dup := seenSrc[s]; dup {
				return nil, fmt.Errorf(
					"golden %s record %d: expected_sources[%d] duplicates %q (id=%s)",
					path, idx, i, s, q.ID,
				)
			}
			seenSrc[s] = struct{}{}
		}
		if q.Type != "" {
			if _, ok := validTypes[q.Type]; !ok {
				return nil, fmt.Errorf(
					"golden %s record %d: unknown type %q (want %q, %q, %q, or empty) (id=%s)",
					path, idx, q.Type, TypeDirect, TypeMultiDoc, TypeAdversarial, q.ID,
				)
			}
		}
		if prev, dup := seen[q.ID]; dup {
			return nil, fmt.Errorf("golden %s record %d: duplicate id %q (first seen record %d)", path, idx, q.ID, prev)
		}
		seen[q.ID] = idx
		queries = append(queries, q)
	}

	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("golden %s: read closing token: %w", path, err)
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("golden %s: no records", path)
	}
	return queries, nil
}
