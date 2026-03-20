# Quickstart

## Prerequisites

[Ollama](https://ollama.com) running locally, with the default models pulled:

```bash
ollama pull llama3.2:1b
ollama pull nomic-embed-text:latest
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

Retrieves the top-5 relevant chunks from the vector store, injects them as context, and streams a RAG-augmented response. If the store is missing or empty, `ask` logs a warning and falls back to the model's own knowledge.

```bash
make build
./bin/go-to-rag ask "What is a Kubernetes operator?"
./bin/go-to-rag ask --model llama3.1:8b "Explain CRDs"
./bin/go-to-rag ask --with-fallback "What does OLM do?"
./bin/go-to-rag --debug ask "What does OLM do?"
```

See [docs/ask.md](ask.md) for all flags and behaviour details.

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

Or run the full pipeline in one shot via Make:

```bash
make run-demo                    # seed, ingest, and ask
make run-demo WITH_FALLBACK=true # same, but allow the model to supplement retrieved context
```

## Flags

`--debug` is the only global flag (available on all subcommands). All other flags : `--host`, `--model`, `--embed-model`, `--db`, `--with-fallback` ,  are per-command. See [docs/ask.md](ask.md) and [docs/ingest.md](ingest.md) for per-command flag references.