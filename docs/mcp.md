# mcp

Start an MCP (Model Context Protocol) server that exposes the RAG pipeline as tools for external LLMs.

```bash
./bin/go-to-rag mcp
```

By default the server starts in retrieval-only mode — no Ollama chat dependency required. Pass `--chat-model` to also enable the LLM chat tool.

## Flags

| Flag            | Default                    | Description                                                                                                                                                                                                                     |
|-----------------|----------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--host`        | `http://localhost:11434`   | Ollama host URL                                                                                                                                                                                                                 |
| `--embed-model` | `mxbai-embed-large:latest` | Embedding model                                                                                                                                                                                                                 |
| `--db`          | `./data/index.db`          | Vector store database path                                                                                                                                                                                                      |
| `--top-k`       | `10`                       | Number of chunks to retrieve per query. Set at startup; not controllable per-request by the calling LLM.                                                                                                                        |
| `--addr`        | *(unset)*                  | HTTP/SSE listen address (e.g. `:8080`); omit for stdio                                                                                                                                                                          |
| `--chat-model`  | *(unset)*                  | Ollama chat model (e.g. `qwen3:1.7b`). When set, the `ask_to_rag_system` tool is registered. When absent, only `check_rag_knowledge_base` is available and no chat dependency is required.                                      |
| `--think`       | `hidden`                   | Default thinking mode for `ask_to_rag_system`: `hidden` (model reasons internally, tokens suppressed), `disabled` (no reasoning), `auto` (model default; surfaces reasoning tokens as a second content item when emitted).      |

## Modes

**Stdio** (default): connects over stdin/stdout. Used by Claude Desktop and the `claude` CLI.

**SSE**: starts an HTTP server when `--addr` is set. Useful for remote or multi-client setups.

Both modes respect SIGINT/SIGTERM and shut down cleanly.

## Tools

### `check_rag_knowledge_base` *(always registered)*

Embeds the question, retrieves the top-k matching chunks from the vector store, and returns raw context separated by `---`. Use this when the calling LLM should reason over the retrieved context itself. Requires only the embed model — no chat model needed.

### `ask_to_rag_system` *(registered only when `--chat-model` is set)*

Retrieves context and generates a synthesised LLM answer in one shot. Accepts an optional `think` parameter per-call to override the server default:

| `think` value | Behaviour |
|---|---|
| *(absent)* | Server default (see `--think` flag; default `hidden`) |
| `hidden` | Model reasons internally; thinking tokens not returned to the caller |
| `disabled` | Reasoning disabled entirely |
| `auto` | Model default; thinking tokens surfaced as a second content item when emitted |

## Claude CLI integration

Build the binary first, then register it. The path must point to the built binary.

```bash
make build

# Retrieval-only (no chat model required)
claude mcp add go-to-rag -- ./bin/go-to-rag mcp

# With chat enabled
claude mcp add go-to-rag -- ./bin/go-to-rag mcp --chat-model qwen3:1.7b

claude mcp list
```

Then use `/mcp` inside a Claude session to verify the server is connected.

To remove:

```bash
claude mcp remove go-to-rag
```