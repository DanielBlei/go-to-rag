# Quickstart

## Prerequisites

[Ollama](https://ollama.com) running locally, with the default models pulled:

```bash
ollama pull llama3.2:1b
ollama pull mxbai-embed-large:latest
```

## Commands

```bash
./bin/go-to-rag ask <prompt>         # ask a question, stream the answer
./bin/go-to-rag seed [directory]     # download documents for ingestion
./bin/go-to-rag ingest [path]        # embed documents into the vector store
./bin/go-to-rag mcp                  # start the MCP server for external LLM integration
./bin/go-to-rag serve                # start the gRPC server on :50051
```

## ask

```bash
./bin/go-to-rag ask <prompt>
```

Retrieves the top `--top-k` chunks (default 10) from the vector store, injects them as context, and streams a RAG-augmented response. If the store is missing or empty, `ask` logs a warning and falls back to the model's own knowledge.

```bash
make build
./bin/go-to-rag ask "What is a Kubernetes operator?"              # standard RAG query
./bin/go-to-rag ask --model llama3.1:8b "Explain CRDs"           # use a larger model
./bin/go-to-rag ask --with-fallback "What does OLM do?"          # retrieved context + model knowledge
./bin/go-to-rag --debug ask "What does OLM do?"                  # log retrieved chunks and prompt
```

See [docs/ask.md](ask.md) for all flags and behaviour details.

## seed

```bash
./bin/go-to-rag seed [directory]
```

Downloads documents to a local directory (default: `./seeds`). Uses a built-in manifest of 14
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

Chunks files, embeds each chunk via Ollama (`mxbai-embed-large`), and stores the result in SQLite.
Already-indexed files are skipped. Default path: `./seeds`.

```bash
./bin/go-to-rag ingest                                         # ./seeds -> ./data/index.db
./bin/go-to-rag ingest ./vault                                 # recurse into any doc tree
./bin/go-to-rag ingest --no-recursive ./docs                   # root directory only
./bin/go-to-rag ingest --glob "*.txt" --db ./custom.db ./docs  # custom extension and db path
```

See [docs/ingest.md](ingest.md) for chunking algorithm, storage schema, and scaling notes.

## mcp

```bash
./bin/go-to-rag mcp
```

Connect your knowledge base to Claude, GPT, or any MCP-compatible LLM. No local inference required.

```bash
make build
claude mcp add go-to-rag -- ./bin/go-to-rag mcp    # register with the Claude CLI
```

Then in a Claude session:

```
> use the go-to-rag tool. How does OLM work in OpenShift?
> use the go-to-rag tool. What is a CRD and how does Kubebuilder use it?
```

See [docs/mcp.md](mcp.md) for all flags, modes, and removal instructions.

## serve

```bash
./bin/go-to-rag serve
```

Starts a gRPC server (default `:50051`) exposing two RPCs:

- **Ask** — streams a RAG-augmented answer token by token
- **RetrieveChunks** — returns scored chunks without generation, for service-to-service use

```bash
./bin/go-to-rag serve                                   # listen on :50051
./bin/go-to-rag serve --grpc-addr :9090                 # custom port
./bin/go-to-rag serve --model llama3.1:8b --top-k 5     # override model and retrieval depth
```

See [docs/serve.md](serve.md) for all flags, RPCs, and grpcurl examples.

## Flags

`--debug` is the only global flag (available on all subcommands). All other flags (`--host`, `--model`, `--embed-model`, `--db`, `--with-fallback`, `--top-k`) are per-command. See [ask.md](ask.md) and [ingest.md](ingest.md) for per-command flag references.