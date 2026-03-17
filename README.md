# go-to-rag

> **Status: under active development — experimental**

A local RAG (Retrieval-Augmented Generation) engine written in Go.

## What it does

- Ingests documents and indexes them using local embeddings via [Ollama](https://ollama.com)
- Retrieves relevant chunks using vector similarity search
- Answers questions using a local LLM (Ollama) — no cloud required
- Designed to be extended: an [MCP](https://modelcontextprotocol.io) interface is planned, allowing any external LLM (Claude, OpenAI, etc.) to use the retrieval engine as a tool while keeping embeddings local

## Quick Start

Requires [Ollama](https://ollama.com) running locally.

```bash
# Pull the default model `llama3.2:1b`, small, fast, great for development
# https://ollama.com/library/llama3.2:1b
ollama pull llama3.2:1b

# Run a demo prompt (What's RAG?)
make run-demo

# Or build and run with your own prompt
make build
./bin/go-to-rag ask "Explain Pod concept in Kubernetes?"

# Change the model
./bin/go-to-rag --model llama3.2 ask "What Kubebuilder does?"

# Download seed documents for the RAG pipeline
# Default manifest includes Kubernetes, OpenShift, OLM, and Kubebuilder docs
./bin/go-to-rag seed                                    # downloads to ./seeds/
./bin/go-to-rag seed --manifest my-docs.yaml ./my-docs  # your own URLs to a custom output
```

## Goals

- Experiment with RAG patterns in Go
- Keep the stack simple and self-hostable (no managed services, no API keys needed to run embeddings)
- Build a clean [Model Context Protocol](https://modelcontextprotocol.io) interface that lets external LLMs use retrieval as a pluggable tool
- Compose local fast retrieval + smart reasoning (local Ollama or remote Claude/OpenAI)

## Stack

| Component | Choice | Why                                                                                                 |
|-----------|--------|-----------------------------------------------------------------------------------------------------|
| Language | Go | Fast, compiled, good ecosystem for systems/tools                                                    |
| Embeddings | Ollama (`nomic-embed-text`) | Local, fast, no API keys, good quality for retrieval                                                |
| Reasoning | Local (Ollama) or remote (Claude/OpenAI via MCP) | Choose based on needs: local keeps everything self-contained, and remote leverages better reasoning |
| Vector store | In-memory (MVP) / Database (planned) | MVP: simplicity + speed. Database: persistence at scale                                             |
| Interface | MCP (Model Context Protocol) | Standard protocol for composing LLM tools, it works with Claude, OpenAI, etc.                       |

## Models

Pre-tuned Ollama Modelfiles are provided in [`modelfiles/`](modelfiles/README.md):

- `llama3.2:1b` — development and fast iteration; CPU-friendly, no GPU required
- `llama3.1:8b` — production-like evaluation; targets a GPU-equipped machine (desktop, laptop, or server)

To switch models, update the `MODELFILE` variable at the top of the `Makefile` to point to the desired Modelfile, then run `make model-delete model-create` to rebuild. 
See [`modelfiles/README.md`](modelfiles/README.md) for VRAM requirements and parameter details.

## Contributing

Any feedback, issues, and ideas are welcome.

## License

[Apache 2.0](LICENSE)