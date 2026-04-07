# ask

Ask a single question and get a streamed, RAG-augmented response.

```bash
./bin/go-to-rag ask <prompt>
```

## Flags

| Flag              | Default                    | Description                                                                               |
|-------------------|----------------------------|-------------------------------------------------------------------------------------------|
| `--host`          | `http://localhost:11434`   | Ollama host URL                                                                           |
| `--model`         | `qwen3:1.7b`               | Chat model                                                                                |
| `--embed-model`   | `mxbai-embed-large:latest` | Embedding model (used only when the store is present)                                     |
| `--db`            | `./data/index.db`          | Vector store database path                                                                |
| `--top-k`         | `10`                       | Number of chunks/top matches to retrieve from the vector store                            |
| `--with-fallback` | `false`                    | Let the model complement the answer with its own knowledge, with or without context found |
| `--think`         | `auto`                     | Thinking token mode: `auto` (model default), `disabled` (no reasoning), `hidden` (reason silently) |

## Workflow

1. **Open store**: opens the SQLite vector store at `--db`. If missing or empty, logs a warning and continues in fallback mode. No error, no exit.
2. **Embed query**: the prompt is embedded via `mxbai-embed-large:latest` and the top `--top-k` chunks are retrieved by cosine similarity.
3. **Build message**: retrieved chunks are joined with `---` separators and injected as a context block.
4. **Generate**: the assembled message is streamed to the chat model. Tokens are written to stdout as they arrive.

## `--think`

Controls how the model's internal reasoning (supported by qwen3 family) is handled:

| Value | Behaviour |
|-------|-----------|
| `auto` | Model default — qwen3 thinks by default; reasoning tokens are printed to stdout in dim gray before the answer |
| `disabled` | Sends `Think=false` to Ollama; model never enters reasoning mode (faster, no `<think>` tokens) |
| `hidden` | Model reasons internally but tokens are suppressed before output; only the final answer is shown |

```bash
# Show reasoning in gray (default)
./bin/go-to-rag ask --model go-to-rag:latest "What does OLM do?"

# Answer only, no visible reasoning
./bin/go-to-rag ask --model go-to-rag:latest --think=hidden "What does OLM do?"

# Disable reasoning entirely (fastest)
./bin/go-to-rag ask --model go-to-rag:latest --think=disabled "What does OLM do?"
```

`--think` is only effective with models that support reasoning tokens (e.g. qwen3). Other models ignore this flag.

## `--with-fallback`

By default, when context is retrieved the model answers strictly from it. No system prompt is injected, letting the Modelfile govern tone and behaviour.

With `--with-fallback`, a system prompt is added that allows the model to draw on its own knowledge and instructs it to note when it does so. Retrieved context is still injected when available. This is a hybrid mode, not a plain fallback.

When no context is found (empty store or missing `--db`), the fallback system prompt is always used regardless of the flag.

## Examples

Requires a populated store. See [quickstart](quickstart.md) to seed and ingest first.

```bash
# Standard RAG query
./bin/go-to-rag ask "What does OLM do?"

# Point at a custom database
./bin/go-to-rag ask --db ./my-index.db "What is a Kubernetes operator?"

# Use a larger chat model
./bin/go-to-rag ask --model llama3.1:8b "Explain CRDs in depth"

# Hybrid, retrieved context injected, model may supplement with own knowledge
./bin/go-to-rag ask --with-fallback "What does OLM do?"

# Debug logs retrieved chunks and prompt assembly
./bin/go-to-rag --debug ask "What is a Kubernetes operator?"
```

Or via Make:

```bash
make run-demo                    # full pipeline + ask
make run-demo WITH_FALLBACK=true # same, hybrid mode
```