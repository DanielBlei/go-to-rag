# eval

Measure retrieval quality before you rely on the pipeline. `eval` ingests a corpus into an ephemeral store, retrieves against a golden query set, and writes a reproducible quality report — no judge model, no external calls beyond the embedding model.

```bash
make eval    # zero-config text report against the bundled K8s corpus
```

Use the metric deltas to guide decisions on chunk size, overlap, embedding model, or source documentation before committing changes.

## Sample report

Output from a real run against the bundled corpus and golden set (`--top-k 10`):

```
Eval Report
  queries: 20 (failed: 0)
  top_k: 10
  embed: mxbai-embed-large:latest
  embed_digest: 468836162de7f81e...
  corpus_hash:  sha256:9b8fbdc461b4ba2d...
  dataset_hash: sha256:a11c26efd46e3cac...

Aggregate
  Hit@K        1.0000
  MRR          1.0000
  Precision@K  0.7500
  Recall@K     0.9500

By query type
  type         n   hit     mrr     p@k     r@k
  adversarial  2   1.0000  1.0000  0.4167  1.0000
  direct       12  1.0000  1.0000  0.7500  1.0000
  multi-doc    6   1.0000  1.0000  0.8611  0.8333

Rank of first hit
  rank=1    20
  rank=2-3  0
  rank=4-K  0
  miss      0

Latency (ms)
  median  21.0
  p95     27.0

Top score (rank-1 similarity)
  min     0.6474
  median  0.7678
  max     0.8761

Per-query
  id    type         hit   rank  rr    p@k   r@k   top    ms  note
  q001  direct       true  1     1.00  1.00  1.00  0.876  25
  q004  direct       true  1     1.00  0.33  1.00  0.757  20
  q013  multi-doc    true  1     1.00  1.00  0.50  0.723  20
  q016  multi-doc    true  1     1.00  0.50  0.50  0.771  25
  q019  adversarial  true  1     1.00  0.33  1.00  0.741  20
  ...
```

The per-type breakdown is where the signal lives. Overall `Recall@K=0.9500` looks strong, but multi-doc queries average 0.83 — two of six retrieved only one of two expected sources. The aggregate alone would not surface this.

Use `--format json` for tooling. The JSON shape separates quality results from runtime diagnostics and includes `higher_is_better` metadata for auto-orient.

## Usage

```bash
./bin/go-to-rag eval [flags]
```

| Flag            | Default                                  | Description                                                      |
|-----------------|------------------------------------------|------------------------------------------------------------------|
| `--dataset`     | `internal/eval/testdata/golden.v1.json`  | Path to a `golden.v1.json` query file                            |
| `--corpus`      | `internal/eval/testdata/corpus`          | Directory of `*.md` files to evaluate against                    |
| `--top-k`       | `5`                                      | Number of chunks to retrieve per query                           |
| `--output`      | _(stdout)_                               | Output path; stdout when empty                                   |
| `--format`      | `json`                                   | Output format: `json` or `text`                                  |
| `--chunk-size`  | `512`                                    | Chunk size in characters                                         |
| `--overlap`     | `100`                                    | Overlap between adjacent chunks                                  |
| `--reuse-db`    | `false`                                  | Persist the embedded corpus to `<work-dir>/run.db` and reuse it  |
| `--work-dir`    | `./.eval-cache`                          | Directory for ephemeral build dirs and the optional reuse cache  |
| `--chat-host`   | `http://localhost:11434`                 | Inference backend host URL (Ollama default)                      |
| `--embed-model` | `mxbai-embed-large:latest`               | Embedding model                                                  |

The chat model is not used in `eval`.

## The golden set

`internal/eval/testdata/golden.v1.json` ships 20 queries across three types: 12 direct (single-source, close phrasing), 6 multi-doc (2+ expected sources), and 2 adversarial (paraphrase and indirect phrasing). The corpus is a frozen K8s documentation snapshot — source URLs and known limitations are in `internal/eval/testdata/SNAPSHOT.md`.

## Metrics

All metrics operate at source granularity with binary relevance: a retrieved chunk is a hit if its source file appears in `expected_sources`, and chunks from the same source dedup before scoring. Source granularity is deliberate — the golden set marks documents, not chunks; chunk-level scoring would incorrectly penalise retrieving multiple chunks from the correct document.

- **Hit@K.** Any expected source in the top-K. Rank-insensitive.
- **MRR.** Reciprocal rank of the first hit, averaged across queries. Rank-sensitive; collapses the distribution to one scalar.
- **Precision@K.** Fraction of unique top-K sources that are expected. Bounded by `1/|sourceSet|` for single-source queries at large K.
- **Recall@K.** Fraction of expected sources retrieved. Bounded by `K/|expected|` for multi-source queries — pick K with that ceiling in mind.
- **`rank_distribution`.** Buckets first-hit rank into: 1, 2-3, 4-K, miss. MRR alone does not show whether near-misses cluster at rank 2 or trail at rank K.

## Reproducibility and caching

Three hashes pin every run:
- `embed_model_digest` — SHA-256 of the Ollama model blob; changes when the model is updated
- `corpus_hash` — SHA-256 over relpath-content pairs of every `*.md` under `--corpus`
- `dataset_hash` — SHA-256 of the golden file

All three appear in the report's `config` block. `run_started_at` is captured as UTC so two reports can be ordered without filesystem timestamps.

By default the SQLite store is built in a tmp directory and removed on exit. Use `--reuse-db` to persist it across runs:

```bash
./bin/go-to-rag eval --reuse-db --work-dir ./.eval-cache
```

The first run writes `run.db` and `run.db.meta.json` under `--work-dir`. Subsequent runs that share the same embed-model digest, corpus hash, chunk size, and overlap reuse the database — any mismatch fails with a message identifying the changed field. The stored database is also useful for manual inspection: `sqlite3 .eval-cache/run.db` with the schema documented in [docs/ingest.md](ingest.md).

## Roadmap

- **LLM Judge.** Score retrieved-context-plus-answer pairs for correctness and faithfulness. Planned as `eval judge` once the assertion baseline is stable.
- **Graded relevance (golden v2).** Per-source relevance scores and negative-control queries (`expected_sources: []`). The v1 schema requires a non-empty expected set, which forecloses off-topic test cases.
- **Human review.** Stratified sampling over assertion/judge disagreements.
