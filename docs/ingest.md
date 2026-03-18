# ingest

Embed documents from a directory into the vector store.

## Usage

```bash
./bin/go-to-rag ingest [path]
```

Default path: `./seeds`

| Flag           | Default           | Description                             |
|----------------|-------------------|-----------------------------------------|
| `--chunk-size` | `512`             | Chunk size in characters (rune-based)   |
| `--overlap`    | `50`              | Overlap between adjacent chunks         |
| `--glob`       | `*.md`            | Glob pattern to match files in `[path]` |
| `--db`         | `./data/index.db` | Vector store database path              |

## Pipeline

For each matched file:

1. **Skip check**: `HasSource` queries SQLite by absolute path. Already-indexed files are skipped entirely.
2. **Chunk**: file is read into memory, converted to `[]rune`, then split with a sliding window: `step = chunkSize - overlap`, producing chunks at offsets `0, step, 2*step, ...`. Whitespace-only chunks are dropped.
3. **Embed**: each chunk is sent to Ollama (`nomic-embed-text`) and returns `[]float32` (768 dimensions).
4. **Store**: embedding is encoded as little-endian bytes (4 bytes × 768 = 3072 bytes) and inserted into SQLite. A failure at any chunk triggers `DeleteSource` to roll back all chunks for that file.

## Chunking

`internal/chunker` operates on runes, not bytes. Multibyte characters are never split.
With defaults (`--chunk-size 512 --overlap 50`), step size is 462 runes. Adjacent chunks share
50 runes of context, which helps retrieval when a relevant sentence straddles a boundary.

### Tuning chunk-size

The embedding model converts a chunk into a single fixed-size vector, so everything inside one
chunk competes for representation in that vector. Smaller chunks are more precise: a 128-rune
chunk about a single concept retrieves well for narrow queries. Larger chunks carry more context
but the embedding becomes diluted. A 1024-rune chunk spanning three topics may match queries
about none of them well.

512 is a reasonable middle ground for technical prose (Markdown docs, READMEs). Go lower (128-256)
for dense reference material where each paragraph is a distinct concept. Go higher (1024+) for
narrative text where ideas span multiple paragraphs and you want them retrieved together.

One practical limit: `nomic-embed-text` has a 512-token context window. A rune is not a token,
but for ASCII-heavy text they are roughly 1:1. Chunks much larger than 512 runes will be silently
truncated by the model, so the tail of a large chunk may not be represented in the embedding.

### Tuning overlap

Overlap is the number of runes repeated between consecutive chunks. It exists to handle boundary
effects: a sentence split across two chunks would be half-represented in each without overlap.

The default of 50 runes is enough to catch a sentence fragment at the end of a chunk. Raising it
(e.g. 100-150) improves retrieval for content where key context tends to appear at chunk edges,
at the cost of storing more chunks and more embedding calls. Setting it to 0 is fine for content
where paragraphs are self-contained and boundaries fall naturally between concepts.

## Storage

SQLite in WAL mode. Schema:

```sql
CREATE TABLE chunks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source      TEXT    NOT NULL,        -- absolute file path
    text        TEXT    NOT NULL,
    embedding   BLOB    NOT NULL,        -- little-endian float32 array
    chunk_index INTEGER NOT NULL,
    UNIQUE(source, chunk_index)
);
CREATE INDEX idx_chunks_source ON chunks(source);
```

- **id**: is an autoincrement surrogate key. Nothing in the application depends on it, but it gives
each row a stable identity if you want to inspect the table manually.

- **source**: is the absolute path of the originating file. It serves two purposes: `HasSource`
queries this column to decide whether a file is already indexed, and search results include it so
the caller knows which document a chunk came from. Using the absolute path keeps the key stable
across working directory changes, but it also means moving source files invalidates their index
entries.

- **text**: is the raw chunk content as a UTF-8 string. It is stored alongside the embedding so
search results can return the actual text without going back to disk. This matters because source
files may be deleted or moved after ingestion. The store is meant to be self-contained.

- **embedding**: is the `[]float32` vector from Ollama, serialised as a little-endian byte array
(4 bytes per float, 3072 bytes total for `nomic-embed-text`'s 768 dimensions). SQLite has no
native float array type, so BLOB is the right fit. Encoding and decoding live in
`internal/vectorstore/sqlite.go` (`encodeEmbedding` / `decodeEmbedding`).

- **chunk_index**: is the zero-based position of the chunk within its source file. Together with
`source` it forms the `UNIQUE` constraint, which prevents double-inserting a chunk if ingest is
run against the same file twice. It is also returned in search results so you can reconstruct
where in the original document a match came from.

The index on `source` makes `HasSource` and `DeleteSource` fast regardless of table size.
Without it, both would be full scans.

Parent directories for `--db` are created automatically (`os.MkdirAll`).

## Search (at query time)

`Search` loads all embeddings from SQLite into memory, computes cosine similarity against the
query vector in Go, then returns the top-N results sorted by score:

```
cosine(a, b) = dot(a, b) / (norm(a) × norm(b))
```

This is a full table scan. Every row is read and scored. At 3072 bytes per chunk, 10k chunks
is ~30 MB resident. The scan is CPU-bound and completes in milliseconds at that scale.

## Scaling

For a larger implementation, swap the backend via the `Store` interface (`internal/vectorstore/store.go`):

```go
type Store interface {
    AddChunk(ctx, source, text string, embedding []float32, chunkIndex int) error
    Search(ctx, queryVec []float32, limit int) ([]Result, error)
    CountChunks(ctx) (int, error)
    HasSource(ctx, source string) (bool, error)
    DeleteSource(ctx, source string) error
    Close() error
}
```

Any struct implementing these 6 methods plugs in with zero changes to `ingest` or `ask`.
Candidates: Qdrant (gRPC), pgvector (SQL), or an HNSW index for ANN search.

## Notes

- Re-running skips already-indexed files; delete the DB to re-index from scratch.
- `--debug` logs per-file chunk count and per-chunk embed progress.
- Requires `nomic-embed-text` reachable via Ollama; chat model is not used.