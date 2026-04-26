# eval

Evaluate your RAG pipeline end to end in a self-contained run. `eval` ingests a corpus into an ephemeral store, retrieves against a golden query set, and writes a reproducible quality report. 

Use the metric deltas to guide decisions on chunk size, overlap, embedding model, or even the source documentation itself before committing changes.

## Usage

```bash
./bin/go-to-rag eval [flags]
```

| Flag            | Default                                  | Description                                                      |
|-----------------|------------------------------------------|------------------------------------------------------------------|
| `--dataset`     | `internal/eval/testdata/golden.v1.json`  | Path to a `golden.v1.json` query file                            |
| `--corpus`      | `internal/eval/testdata/corpus`          | Directory of `*.md` files to evaluate against                    |
| `--top-k`       | `5`                                      | Number of chunks to retrieve per query                           |
| `--output`      | _(stdout)_                               | Output path, stdout when empty                                   |
| `--format`      | `json`                                   | Output format, `json` or `text`                                  |
| `--chunk-size`  | `512`                                    | Chunk size in characters                                         |
| `--overlap`     | `100`                                    | Overlap between adjacent chunks                                  |
| `--reuse-db`    | `false`                                  | Persist the embedded corpus to `<work-dir>/run.db` and reuse it  |
| `--work-dir`    | `./.eval-cache`                          | Directory for ephemeral build dirs and the optional reuse cache  |
| `--host`        | `http://localhost:11434`                 | Ollama host URL                                                  |
| `--embed-model` | `mxbai-embed-large:latest`               | Ollama embedding model                                           |

The chat model is not used in `eval`.

## The golden set

`internal/eval/testdata/golden.v1.json` ships 20 queries across three types: 12 direct (single-source, close phrasing), 6 multi-doc (2+ expected sources), and 2 adversarial (paraphrase and indirect phrasing). 

The corpus is a frozen K8s documentation snapshot. Source URLs and known limitations are documented in `internal/eval/testdata/SNAPSHOT.md`.

## Evaluation Goals

Running `eval` validates retrieval quality at source granularity (documents, not chunks). Output is reproducible across machines and runs, providing concrete metrics for fine-tuning decisions.

The harness is the first of three evaluation layers:

- **Assertion-based (current).** Binary relevance, no judge, fully reproducible. Measures whether the right documents surface, not whether the answer was correct.
- **LLM Judge (planned).** Correctness and faithfulness scoring over retrieved-context-plus-answer pairs. Can run against Claude or a local model via Ollama.
- **Human review (planned).** Manual review of assertion/judge disagreements.

## Metrics

All four metrics operate at source granularity with binary relevance: a retrieved chunk is a hit if its source file appears in `expected_sources`, and chunks from the same source dedup before scoring. 

Source granularity is deliberate: the golden set marks documents, not chunks. Chunk-level would incorrectly penalise retrieving multiple chunks from the correct document.

- **Hit@K.** Any expected source in the top-K. Rank-insensitive. (Scored at document granularity, so not directly comparable to chunk-level baselines.)
- **MRR.** Reciprocal rank of the first hit, averaged across queries. Rank-sensitive, collapses the distribution to one scalar.
- **Precision@K.** Fraction of unique top-K sources that are expected. Bounded by `1/|sourceSet|` for single-source queries at large K.
- **Recall@K.** Fraction of expected sources retrieved. Bounded by `K/|expected|` for multi-source queries. Pick K with that ceiling in mind.
- **`rank_distribution`.** Buckets first-hit rank into: 1, 2-3, 4-K, miss. MRR alone does not say whether near-misses cluster at rank 2 or trail at rank K. The histogram makes that visible.

## Report

`eval` produces a per-query report so the numbers that matter don't hide behind a single aggregate. 

In the run below, overall Recall looks strong at 0.95, but the per-type breakdown shows multi-doc queries averaging 0.83. Two of six multi-doc queries scored `r@k=0.50`, each retrieving one of two expected sources. The top-level `Recall@K=0.9500` does not show it.

A few more signals the report makes visible:

- `rank_1=20` confirms MRR=1.0 is genuine, not an average smoothing over rank-2 near-misses.
- Adversarial `p@k=0.4167` at k=10 with perfect recall means the right documents are being found, but the model pulls in noise at wider k. Tightening k or adjusting chunk size would improve precision without touching recall here.
- Similarity `min=0.6474` across all queries suggests the corpus is topically tight enough that nothing is being retrieved by chance.

Output from a real run against the bundled corpus and golden set (`--top-k 10`):

```bash
./bin/go-to-rag eval --top-k 10 --format text
```

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

Use `--format json` for tooling. The JSON shape separates quality results from runtime diagnostics and includes `higher_is_better` auto-orient metadata.

## Reproducibility and caching

Three hashes pin every run: 
- `embed_model_digest` (sha256 of the Ollama model blob, changes when the model is updated)
- `corpus_hash` (sha256 over relpath-content pairs of every `*.md` under `--corpus`, changes when any document is edited)
- `dataset_hash` (sha256 of the golden file)

All three appear in the report's `config` block. `run_started_at` is captured as UTC so two reports can be ordered without filesystem timestamps.

By default, the SQLite store is built in a tmp directory and removed on exit. Use `--reuse-db` to persist it:

```bash
./bin/go-to-rag eval --reuse-db --work-dir ./.eval-cache
```

The first run writes `run.db` and `run.db.meta.json` under `--work-dir`. Subsequent runs that share the same embed-model digest, corpus hash, chunk size, and overlap reuse the database. 

Any mismatch fails, highlighting the changed field and a `delete <work-dir> to rebuild` hint. Keeping the database is also useful for manually inspecting retrieved chunks: `sqlite3 .eval-cache/run.db` with the schema in `docs/ingest.md`.

## Roadmap

- **LLM Judge.** Score retrieved-context-plus-answer pairs for correctness and faithfulness. Planned as a separate `eval judge` subcommand once assertion-layer numbers are stable.
- **Graded relevance (golden v2).** Extend the schema with per-source relevance scores and support for negative-control queries (`expected_sources: []`). The v1 schema requires a non-empty expected set, which forecloses off-topic test cases.
- **Human review.** Stratified sampling over assertion/judge disagreements. Design not yet defined.
