# mcp

Start an MCP (Model Context Protocol) server that exposes the RAG pipeline as a tool for external LLMs.

```bash
./bin/go-to-rag mcp
```

## Flags

| Flag            | Default                   | Description                                                                                                                                                                                                                     |
|-----------------|---------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--host`        | `http://localhost:11434`  | Ollama host URL                                                                                                                                                                                                                 |
| `--embed-model` | `mxbai-embed-large:latest` | Embedding model                                                                                                                                                                                                                 |
| `--db`          | `./data/index.db`         | Vector store database path                                                                                                                                                                                                      |
| `--top-k`       | `10`                      | Number of chunks/top matches to retrieve per query, set at server startup, not controllable per-request by the calling LLM. Re-register with a lower value (e.g. `--top-k 5`) for a tighter, more deterministic context window. |
| `--addr`        | *(unset)*                 | HTTP/SSE listen address (e.g. `:8080`); omit for stdio                                                                                                                                                                          |

## Modes

**Stdio** (default): connects over stdin/stdout. Used by Claude desktop and the `claude` CLI.

**SSE**: starts an HTTP server when `--addr` is set. Useful for remote or multi-client setups.

Both modes respect SIGINT/SIGTERM and shut down cleanly.

## Tool: `ask_to_rag_system`

The server exposes a single tool. Pass the user's question as-is; the tool embeds it, retrieves the top-k matching chunks, and returns them as context separated by `---`.

## Claude CLI integration

Build the binary first, then register it. The path must point to the built binary.

```bash
make build
claude mcp add go-to-rag -- ./bin/go-to-rag mcp
claude mcp list
```

Then use `/mcp` inside a Claude session to verify the server is connected.

To remove:

```bash
claude mcp remove go-to-rag
```