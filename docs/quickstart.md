# Quickstart

Get from zero to your first RAG-augmented answer in under five minutes.

## Prerequisites

**Ollama (default)**

[Ollama](https://ollama.com) running locally, with the default models pulled:

```bash
ollama pull qwen3:1.7b
ollama pull mxbai-embed-large:latest
```

**vLLM (alternative)**

A [vLLM](https://docs.vllm.ai) server with a chat model and an embedding model loaded. See [docs/vllm.md](vllm.md) for setup, model naming, and all command examples.

## One-shot demo

The fastest path — seeds the bundled K8s/OLM knowledge base, embeds it, and answers a question:

```bash
make build && make run-demo
```

## Step-by-step pipeline

Each stage is independently composable:

```bash
make build

# 1. Download K8s/OLM/OpenShift docs to ./seeds
./bin/go-to-rag seed

# 2. Chunk, embed, and index into SQLite at ./data/index.db
./bin/go-to-rag ingest

# 3. Retrieve context and stream an answer
./bin/go-to-rag ask "What does OLM do?"
```

`ask` embeds your question, retrieves the top-10 most similar chunks, injects them as context, and streams the model's answer to stdout. If the store is missing or empty, it falls back to the model's own knowledge and logs a warning.

## Using your own documents

Point `seed` at a custom manifest of URLs, or skip it entirely and ingest any local directory:

```bash
# Ingest a local directory of Markdown files
./bin/go-to-rag ingest ./my-docs

# Download from a custom URL list, then ingest
./bin/go-to-rag seed --manifest urls.yaml ./my-docs
./bin/go-to-rag ingest ./my-docs

# Ask against your corpus
./bin/go-to-rag ask "What is the retention policy for audit logs?"
```

See [docs/seed.md](seed.md) for the manifest format.

## Next steps

- **Tune the model** — [`modelfiles/README.md`](../modelfiles/README.md) has pre-tuned Modelfiles that add a RAG-specific system prompt. `make model-create` builds `go-to-rag:latest`.
- **vLLM** — shared-GPU or production deployments: [docs/vllm.md](vllm.md)
- **MCP** — connect to Claude or any MCP-compatible LLM as a tool: [docs/mcp.md](mcp.md)
- **gRPC server** — service-to-service integration: [docs/serve.md](serve.md)
- **Retrieval eval** — measure and iterate on retrieval quality: [docs/eval.md](eval.md)
