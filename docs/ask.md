# ask

Ask a single question and get a streamed, RAG-augmented response.

```bash
./bin/go-to-rag ask <prompt>
```

## Flags

| Flag              | Default                   | Description                                                                               |
|-------------------|---------------------------|-------------------------------------------------------------------------------------------|
| `--host`          | `http://localhost:11434`  | Ollama host URL                                                                           |
| `--model`         | `llama3.2:1b`             | Chat model                                                                                |
| `--embed-model`   | `nomic-embed-text:latest` | Embedding model (used only when the store is present)                                     |
| `--db`            | `./data/index.db`         | Vector store database path                                                                |
| `--with-fallback` | `false`                   | Let the model complement the answer with its own knowledge, with or without context found |

## Workflow

1. **Open store**: opens the SQLite vector store at `--db`. If missing or empty, logs a warning and continues in fallback mode ,  no error, no exit.
2. **Embed query**: the prompt is embedded via `nomic-embed-text:latest` and the top-5 chunks are retrieved by cosine similarity.
3. **Build message**: retrieved chunks are joined with `---` separators and injected as a context block.
4. **Generate**: the assembled message is streamed to the chat model. Tokens are written to stdout as they arrive.

## `--with-fallback`

By default, when context is retrieved the model answers strictly from it ,  no system prompt is injected, letting the Modelfile govern tone and behaviour.

With `--with-fallback`, a system prompt is added that allows the model to draw on its own knowledge and instructs it to note when it does so. Retrieved context is still injected when available ,  this is a hybrid mode, not a plain fallback.

When no context is found (empty store or missing `--db`), the fallback system prompt is always used regardless of the flag.

## Examples

```bash
# Standard RAG query (requires a populated store)
make build
./bin/go-to-rag seed
./bin/go-to-rag ingest
./bin/go-to-rag ask "What does OLM do?"

# Point at a custom database
./bin/go-to-rag ask --db ./my-index.db "What is a Kubernetes operator?"

# Use a larger chat model
./bin/go-to-rag ask --model llama3.1:8b "Explain CRDs in depth"

# Hybrid — retrieved context injected, model may supplement with own knowledge
./bin/go-to-rag ask --with-fallback "What does OLM do?"

# Debug — logs retrieved chunks and prompt assembly
./bin/go-to-rag --debug ask "What is a Kubernetes operator?"
```

Or via Make:

```bash
make run-demo                    # full pipeline + ask
make run-demo WITH_FALLBACK=true # same, hybrid mode
```