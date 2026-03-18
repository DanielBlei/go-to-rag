# Quickstart

## Prerequisites

[Ollama](https://ollama.com) running locally, with the default models pulled:

```bash
ollama pull llama3.2:1b
ollama pull nomic-embed-text
```

## Commands

```bash
go-to-rag ask <prompt>         # ask a question, stream the answer
go-to-rag seed [directory]     # download documents for ingestion
go-to-rag ingest [path]        # embed documents into the vector store
```

## ask

```bash
./bin/go-to-rag ask <prompt>
```

Streams a response from the chat model to stdout. No vector store needed.

```bash
make build
./bin/go-to-rag ask "What is a Kubernetes operator?"
./bin/go-to-rag --model llama3.2 ask "Explain CRDs"
./bin/go-to-rag --debug ask "What does OLM do?"
```

`ask` only checks that the chat model is reachable and does not require `nomic-embed-text`.

## seed

```bash
./bin/go-to-rag seed [directory]
```

Downloads documents to a local directory (default: `./seeds`). Uses a built-in manifest of 12
K8s/OLM/OpenShift/Kubebuilder docs. Pass `--manifest` to use your own URL list.

```bash
./bin/go-to-rag seed                                   # downloads to ./seeds/
./bin/go-to-rag seed ./my-docs                         # custom output dir
./bin/go-to-rag seed --manifest urls.yaml ./my-docs    # custom manifest + dir
```

Existing files are skipped. Does not require Ollama.

See [docs/seed.md](seed.md) for manifest format and default corpus details.

## ingest

```bash
./bin/go-to-rag ingest [path]
```

Chunks files, embeds each chunk via Ollama (`nomic-embed-text`), and stores the result in SQLite.
Already-indexed files are skipped. Default path: `./seeds`.

```bash
./bin/go-to-rag ingest                                          # ./seeds -> ./data/index.db
./bin/go-to-rag ingest ./my-docs                                # custom source dir
./bin/go-to-rag ingest --chunk-size 256 --overlap 32 ./my-docs
./bin/go-to-rag ingest --glob "*.txt" --db ./custom.db ./my-docs
```

See [docs/ingest.md](ingest.md) for chunking algorithm, storage schema, and scaling notes.

## Full pipeline

```bash
make build
./bin/go-to-rag seed
./bin/go-to-rag ingest
./bin/go-to-rag ask "What does OLM do?"
```

## Global flags

| Flag            | Default                   | Description                |
|-----------------|---------------------------|----------------------------|
| `--host`        | `http://localhost:11434`  | Ollama host URL            |
| `--model`       | `llama3.2:1b`             | Chat model                 |
| `--embed-model` | `nomic-embed-text`        | Embedding model            |
| `--db`          | `./data/index.db`         | Vector store database path |
| `--debug`       | `false`                   | Enable debug logging       |