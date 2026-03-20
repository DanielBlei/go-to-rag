# go-to-rag

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)
![Ollama](https://img.shields.io/badge/Ollama-local-black?style=flat&logo=ollama&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-WAL-003B57?style=flat&logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-Apache%202.0-blue?style=flat)

A local RAG (Retrieval-Augmented Generation) engine written in Go, powered by [Ollama](https://ollama.com).

Seed documents, embed them locally, and ask questions with no cloud and no API keys required.

## Status

The core RAG pipeline (`seed -> ingest -> ask`) is functional. Multi-turn chat and an [MCP](https://modelcontextprotocol.io) server interface are planned.

## Requirements

- Go 1.22+
- [Ollama](https://ollama.com) 0.5+, running locally
- Models pulled:

```bash
ollama pull llama3.2:1b
ollama pull nomic-embed-text:latest
```

## Quick start

```bash
make build

# Seed K8s docs, embed, and ask a question in one shot
make run-demo

# Or step through the pipeline manually
./bin/go-to-rag seed
./bin/go-to-rag ingest
./bin/go-to-rag ask "What does OLM do?"
```

See [docs/quickstart.md](docs/quickstart.md) for the full pipeline walkthrough and flag reference.

## Commands

| Command | Description |
|---------|-------------|
| `ask <prompt>` | RAG-augmented question, retrieves relevant chunks and streams the answer |
| `seed [dir]` | Download K8s/OLM/OpenShift docs for ingestion (default: `./seeds`) |
| `ingest [path]` | Chunk, embed, and index documents into SQLite (default: `./seeds`) |

## Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go | Fast, compiled, good fit for systems tooling |
| Embeddings | `nomic-embed-text:latest` via Ollama | Local, no API keys, 768-dim vectors |
| Vector store | SQLite (WAL mode) | Zero-dependency MVP; swappable via `Store` interface |
| Chat | Ollama (local) | Self-contained; MCP interface for remote LLMs planned |
| CLI | Cobra | Subcommand structure with per-command flags |

## Models

Pre-tuned Modelfiles optimised for factual, grounded RAG output are in [`modelfiles/`](modelfiles/README.md):

| Model | Modelfile | Use case |
|-------|-----------|----------|
| `llama3.2:1b` | `llama3.2-1b.Modelfile` | Development, CPU-friendly, fast iteration |
| `llama3.1:8b` | `llama3.1-8b.Modelfile` | Production-like evaluation, GPU recommended |

To switch models, update the `MODELFILE` variable at the top of the `Makefile` to point to the desired Modelfile, then run `make model-create` to rebuild.
See [`modelfiles/README.md`](modelfiles/README.md) for VRAM requirements and parameter details.

```bash
make model-create   # build (or rebuild) the custom model from the active Modelfile
```

## Development

```bash
make test      # go test -race -count=1 ./...
make lint      # golangci-lint run ./...
make build     # builds to bin/go-to-rag
make fmt       # gofmt -w .
```

## Contributing

Any feedback, issues, and ideas are welcome.

## License

[Apache 2.0](LICENSE)