# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build        # Build binary to bin/go-to-rag
make test         # go test -race -count=1 ./...
make lint         # golangci-lint run ./...
make lint-fix     # golangci-lint run --fix ./...
make fmt          # gofmt -w .
make tidy         # go mod tidy
make run-demo     # Build and run with a demo prompt
make clean        # Remove bin/
```

Run a single test package:
```bash
go test -race -count=1 ./internal/ollama/...
```

Run the binary directly (requires Ollama running at `http://localhost:11434`):
```bash
./bin/go-to-rag [flags] "<prompt>"
./bin/go-to-rag -model llama3.2 -debug "what is RAG?"
```

## Architecture

This is an early-stage local RAG (Retrieval-Augmented Generation) engine. The planned pipeline is: ingest documents → embed chunks → store vectors → retrieve relevant chunks → augment prompt → generate answer. Currently only the Ollama client (embed + chat) is wired up; the vector store and retrieval layers are not yet implemented.

**Entry point** — `main.go`: Parses flags (`-host`, `-model`, `-embed-model`, `-debug`), sets up signal-based graceful shutdown via `withSignalCancel`, validates Ollama connectivity, then calls `client.Chat`. The embed model check is disabled (`checkEmbed=false`) until the vector store is wired in.

**`internal/ollama`** — wraps `github.com/ollama/ollama/api`:
- `New(host, embedModel, chatModel)` — creates the client
- `Validate(ctx, checkEmbed, checkChat)` — confirms Ollama is reachable and required models are pulled; result is cached via `validated` flag
- `Embed(ctx, text)` — returns `[]float32` embeddings via `nomic-embed-text`
- `Chat(ctx, prompt, w)` — streams single-turn response tokens to `w`; uses a 3-minute timeout (vs 30s for all other calls)

**`internal/logger`** — thin zerolog wrapper; outputs to stderr with timestamps; debug level toggled via `-debug` flag.

## Conventions

- Import grouping (enforced by goimports): stdlib → external → `github.com/DanielBlei/go-to-rag/...`
- All errors are wrapped with `fmt.Errorf("context: %w", err)`
- Structured logging via `zerolog` — use the package-level `log` in `main.go` or accept a `zerolog.Logger` parameter in internal packages
- Default models: embed=`nomic-embed-text`, chat=`llama3.2:1b`

## Planned Work

- Vector store (in-memory MVP, then persistent)
- Document ingestion and chunking pipeline
- MCP (Model Context Protocol) interface so external LLMs (Claude, OpenAI) can use retrieval as a tool