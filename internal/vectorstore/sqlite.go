package vectorstore

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS chunks (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	source      TEXT    NOT NULL,
	text        TEXT    NOT NULL,
	embedding   BLOB    NOT NULL,
	chunk_index INTEGER NOT NULL,
	UNIQUE(source, chunk_index)
);
CREATE INDEX IF NOT EXISTS idx_chunks_source ON chunks(source);
`

const (
	pragmaWAL         = "PRAGMA journal_mode=WAL"
	searchQuery       = "SELECT source, text, chunk_index, embedding FROM chunks"
	insertChunkQuery  = "INSERT INTO chunks (source, text, embedding, chunk_index) VALUES (?, ?, ?, ?)"
	countChunksQuery  = "SELECT COUNT(*) FROM chunks"
	hasSourceQuery    = "SELECT COUNT(*) FROM chunks WHERE source = ?"
	deleteSourceQuery = "DELETE FROM chunks WHERE source = ?"
)

// SQLiteStore is a SQLite-backed vector store.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLite opens (or creates) a SQLite database at dbPath and runs migrations.
func NewSQLite(dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec(pragmaWAL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// AddChunk stores a chunk with its embedding.
func (s *SQLiteStore) AddChunk(ctx context.Context, source, text string, embedding []float32, chunkIndex int) error {
	blob := encodeEmbedding(embedding)
	_, err := s.db.ExecContext(ctx, insertChunkQuery, source, text, blob, chunkIndex)
	if err != nil {
		return fmt.Errorf("insert chunk: %w", err)
	}
	return nil
}

// Search returns the top-limit chunks most similar to queryVec.
func (s *SQLiteStore) Search(ctx context.Context, queryVec []float32, limit int) ([]Result, error) {
	rows, err := s.db.QueryContext(ctx, searchQuery)
	if err != nil {
		return nil, fmt.Errorf("query chunks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type scored struct {
		Result
		score float64
	}

	var candidates []scored
	for rows.Next() {
		var (
			source     string
			text       string
			chunkIndex int
			blob       []byte
		)
		if err := rows.Scan(&source, &text, &chunkIndex, &blob); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		emb := decodeEmbedding(blob)
		score := cosineSimilarity(queryVec, emb)
		candidates = append(candidates, scored{
			Result: Result{Source: source, Text: text, ChunkIndex: chunkIndex},
			score:  score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if limit > len(candidates) {
		limit = len(candidates)
	}
	results := make([]Result, limit)
	for i := 0; i < limit; i++ {
		r := candidates[i].Result
		r.Score = candidates[i].score
		results[i] = r
	}
	return results, nil
}

// CountChunks returns the total number of stored chunks.
func (s *SQLiteStore) CountChunks(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, countChunksQuery).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count chunks: %w", err)
	}
	return count, nil
}

// HasSource reports whether any chunks exist for the given source.
func (s *SQLiteStore) HasSource(ctx context.Context, source string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, hasSourceQuery, source).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("has source: %w", err)
	}
	return count > 0, nil
}

// DeleteSource removes all chunks for the given source.
func (s *SQLiteStore) DeleteSource(ctx context.Context, source string) error {
	_, err := s.db.ExecContext(ctx, deleteSourceQuery, source)
	if err != nil {
		return fmt.Errorf("delete source: %w", err)
	}
	return nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// encodeEmbedding serialises float32 slice as little-endian bytes.
func encodeEmbedding(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// decodeEmbedding deserialises little-endian bytes into a float32 slice.
func decodeEmbedding(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// cosineSimilarity computes dot(a,b) / (norm(a) * norm(b)).
// Returns 0 for zero vectors.
// TODO: replace with sqlite-vec or similar ANN index
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	denom := math.Sqrt(normA * normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
