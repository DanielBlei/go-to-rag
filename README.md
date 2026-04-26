# go-to-rag

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go&logoColor=white)
![Ollama](https://img.shields.io/badge/Ollama-local-black?style=flat&logo=ollama&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-WAL-003B57?style=flat&logo=sqlite&logoColor=white)
![MCP](https://img.shields.io/badge/MCP-compatible-6B4FBB?style=flat&logo=anthropic&logoColor=white)
![gRPC](https://img.shields.io/badge/gRPC-50051-244c5a?style=flat&logo=grpc)
![License](https://img.shields.io/badge/License-Apache%202.0-blue?style=flat)

A local RAG (Retrieval-Augmented Generation) engine written in Go, powered by [Ollama](https://ollama.com). 

Seed, embed, and query a knowledge base entirely on-device. 

Access the pipeline through the CLI, connect it to Claude or any MCP-compatible LLM via the built-in [MCP](docs/mcp.md) server, or integrate service-to-service over [gRPC](docs/serve.md) with native token streaming.

## Requirements

- Go 1.25+
- [Ollama](https://ollama.com) 0.5+, running locally
- Models pulled:

```bash
ollama pull qwen3:1.7b                # default chat model
ollama pull mxbai-embed-large:latest  # embedding model
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
| `eval` | Assert retrieval quality against a golden query set and produce a reproducible report |
| `mcp` | Start the MCP server for external LLM integration (stdio by default, SSE with `--addr`) |
| `serve` | Start the gRPC server (default `:50051`); exposes `Ask` (streaming) and `RetrieveChunks` RPCs |

## Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go | Fast, compiled, good fit for systems tooling |
| Embeddings | `mxbai-embed-large:latest` via Ollama | Local, no API keys, 1024-dim vectors |
| Vector store | SQLite (WAL mode) | Zero-dependency MVP; swappable via `Store` interface |
| Chat | Ollama (local) | Self-contained, fully local inference |
| MCP SDK | [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) | Official Go MCP SDK for tool registration, stdio and SSE transport |
| gRPC | `google.golang.org/grpc` + protobuf | RPC interface for service-to-service and programmatic access |
| Protobuf | `buf` CLI | Schema definition, linting, and Go stub generation |
| CLI | Cobra | Subcommand structure with per-command flags |

## Safety

Two layers of indirect prompt injection mitigation (OWASP LLM02):

- **`InjectionGuard`** - `ChatServer` decorator applied automatically in `Ask()`. 
Sanitises embedded sentinel strings, frames the context block with trust boundary markers, and appends an untrusted-data notice to the system prompt. Covers all entry points with no per-transport opt-in.
- **MCP structured envelope** - `check_rag_knowledge_base` returns a JSON object with a `_data_notice` sentinel and per-chunk attribution and confidence scores rather than raw text.

> **Note:**  Sentinel strings are fixed in source, this mitigates naïve injection, not targeted attacks. 
A per-request nonce would be stronger and is a known future direction. Prompt guardrails (input/output validation pass) would follow the same `ChatServer` decorator pattern.

## Models

The default chat model (`qwen3:1.7b`) is balanced for speed and quality on development hardware. Additional pre-tuned Modelfiles are in [`modelfiles/`](modelfiles/README.md).

`make model-create` rebuilds `go-to-rag:latest` from the `MODELFILE` set in the Makefile, used by `run-demo`. 

For any CLI command, pass `--model` directly instead. See [Quickstart](#quickstart) and [`modelfiles/`](modelfiles/README.md).

> **Note:** Out-of-the-box Ollama models aren't tuned for RAG. For best results, use one of the provided Modelfiles, which prioritise answering questions.

## Evaluation

`eval` measures retrieval quality before you rely on the pipeline. It runs assertion-based metrics against a frozen corpus and a golden query set: no judge, no external calls beyond the embedding model, and the same inputs always produce the same numbers. Use it to validate changes to chunk size, overlap, or embedding model before committing them.

An LLM Judge tier for correctness and faithfulness scoring is planned once the assertion baseline is stable. See [docs/eval.md](docs/eval.md) for usage, metrics, and the full methodology.

## Docker

Build and run the full pipeline in a container (requires Ollama on the host). Auto-detects podman (Default) or docker:

```bash
make docker-demo

# Override the prompt or force a specific runtime:
make docker-demo DEMO_PROMPT="What is a CRD?"
make docker-demo CONTAINER_TOOL=docker
```

## Project Roadmap

### Shared inference backend (vLLM) 

vLLM is the recommended inference layer for production multi-agent deployments (see below).

One model served across all agents, with concurrent fan-out handled efficiently via continuous batching — vLLM processes requests from multiple agents together rather than sequentially, which is the primary advantage over per-agent Ollama processes under load.
It also allows running a single, larger model rather than loading multiple smaller instances per agent, making better use of available GPU resources.

vLLM's OpenAI-compatible endpoint makes it a drop-in replacement at the client level, no changes to agent logic required.

> **Note:** For development purposes, Ollama is the default

### Multi-agent Compose

Domain-scoped RAG agents behind a router with concurrent fan-out queries.

The gRPC layer provides the service-to-service backbone: each domain agent is a
`go-to-rag` instance serving its own knowledge base over gRPC, and the router
fans out queries to all agents in parallel, merging their streamed responses.


Access the router through any entry point - CLI, MCP, or gRPC. 
Under the hood, the router fans out to each domain agent over gRPC, querying their knowledge bases in parallel and merging the results.

```mermaid
graph TB
    User["👤 User: CLI"]
    LLM["🤖 AI Assistant: e.g Claude Code via MCP"]
    Machine["⚙️ Service: gRPC"]

    R["Router"]

    A["go-to-rag<br/>Knowledge A<br/>:50051"]
    B["go-to-rag<br/>Knowledge B<br/>:50052"]
    C["go-to-rag<br/>Knowledge C<br/>:50053"]
    
    V["vLLM<br/>Shared Inference<br/>:8000"]

    User --> R
    LLM --> R
    Machine --> R
    R <--> A
    R <--> B
    R <--> C
    
    A -.-> V
    B -.-> V
    C -.-> V

    style User fill:#6B4FBB,color:#fff
    style LLM fill:#6B4FBB,color:#fff
    style Machine fill:#6B4FBB,color:#fff
    style R fill:#2d6a4f,color:#fff
    style A fill:#244c5a,color:#fff
    style B fill:#244c5a,color:#fff
    style C fill:#244c5a,color:#fff
    style V fill:#8B6F47,color:#fff
```

## Contributing

Issues and PRs welcome. Use [GitHub Issues](https://github.com/DanielBlei/go-to-rag/issues) for bugs, features, and questions.

## License

[Apache 2.0](LICENSE)
