# go-to-rag

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)
![Ollama](https://img.shields.io/badge/Ollama-local-black?style=flat&logo=ollama&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-WAL-003B57?style=flat&logo=sqlite&logoColor=white)
![MCP](https://img.shields.io/badge/MCP-compatible-6B4FBB?style=flat&logo=anthropic&logoColor=white)
![License](https://img.shields.io/badge/License-Apache%202.0-blue?style=flat)

A local RAG (Retrieval-Augmented Generation) engine written in Go, powered by [Ollama](https://ollama.com).

Run fully local with Ollama, or connect your knowledge base to Claude, GPT, or any MCP-compatible LLM via the built-in [MCP](https://modelcontextprotocol.io) server.

## Requirements

- Go 1.22+
- [Ollama](https://ollama.com) 0.5+, running locally
- Models pulled:

```bash
ollama pull llama3.2:1b
ollama pull nomic-embed-text:latest
```

## Quick start

Once the models are pulled (see Requirements above), seed a K8s/OLM/OpenShift knowledge base and ask your first question in one shot:

```bash
make run-demo    # seed docs, embed into SQLite, and ask a question
```

Or step through the pipeline manually:

```bash
make build
./bin/go-to-rag seed                      # download K8s/OLM/OpenShift docs to ./seeds
./bin/go-to-rag ingest                    # chunk, embed, and index into SQLite
./bin/go-to-rag ask "What does OLM do?"   # retrieve context and stream the answer
```

See [docs/quickstart.md](docs/quickstart.md) for the full pipeline walkthrough and flag reference.

## Commands

| Command | Description |
|---------|-------------|
| `ask <prompt>` | RAG-augmented question, retrieves relevant chunks and streams the answer |
| `seed [dir]` | Download K8s/OLM/OpenShift docs for ingestion (default: `./seeds`) |
| `ingest [path]` | Chunk, embed, and index documents into SQLite (default: `./seeds`) |
| `mcp` | Start the MCP server for external LLM integration (stdio by default, SSE with `--addr`) |

## Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go | Fast, compiled, good fit for systems tooling |
| Embeddings | `nomic-embed-text:latest` via Ollama | Local, no API keys, 768-dim vectors |
| Vector store | SQLite (WAL mode) | Zero-dependency MVP; swappable via `Store` interface |
| Chat | Ollama (local) | Self-contained, fully local inference |
| MCP SDK | [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) | Official Go MCP SDK for tool registration, stdio and SSE transport |
| CLI | Cobra | Subcommand structure with per-command flags |

## Models

Pre-tuned Modelfiles for the built-in K8s/OLM/OpenShift/Kubebuilder knowledge base are in [`modelfiles/`](modelfiles/README.md):

| Model | Modelfile | Use case |
|-------|-----------|----------|
| `llama3.2:1b` | `llama3.2-1b.Modelfile` | Development, CPU-friendly, fast iteration |
| `llama3.1:8b` | `llama3.1-8b.Modelfile` | Production-like evaluation, GPU recommended |

To switch models, update the `MODELFILE` variable at the top of the `Makefile` to point to the desired Modelfile, then run `make model-create` to rebuild.
See [`modelfiles/README.md`](modelfiles/README.md) for VRAM requirements and parameter details.

## Docker

Build and run the full pipeline in a container (requires Ollama on the host). Auto-detects podman (Default) or docker:

```bash
make docker-demo

# Override the prompt or force a specific runtime:
make docker-demo DEMO_PROMPT="What is a CRD?"
make docker-demo CONTAINER_TOOL=docker
```

## Development

```bash
make help    # list all available targets
make build   # build the binary
make test    # run tests
```

## Project Roadmap

- **gRPC API** -- expose the retrieval pipeline over gRPC as an alternative to CLI and MCP
- **Multi-agent Compose** -- domain-scoped RAG agents behind a router with concurrent fan-out queries

## Contributing

Issues and PRs welcome. Use [GitHub Issues](https://github.com/DanielBlei/go-to-rag/issues) for bugs, features, and questions.

## License

[Apache 2.0](LICENSE)
