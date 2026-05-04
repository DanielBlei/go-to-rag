# vLLM Backend

vLLM provides an OpenAI-compatible HTTP API, making it suitable for shared-GPU or multi-tenant deployments where Ollama's single-user local model is not appropriate.

## How it works

go-to-rag uses three vLLM API endpoints:

| Endpoint | Used by |
|----------|---------|
| `GET /v1/models` | Startup validation |
| `POST /v1/embeddings` | `ingest`, `ask`, `eval` |
| `POST /v1/chat/completions` | `ask`, `mcp`, `serve` |

vLLM serves **one model per process**. Embedding models and chat models run as separate `vllm serve` processes on separate ports, which is why `--embed-host` exists as a distinct flag from `--chat-host`.

## Model naming

vLLM uses HuggingFace model IDs, not Ollama's `name:tag` format:

| Ollama | vLLM |
|--------|------|
| `qwen3:1.7b` | `Qwen/Qwen3-1.7B` |
| `qwen3:8b` | `Qwen/Qwen3-8B` |
| `llama3.1:8b` | `meta-llama/Llama-3.1-8B-Instruct` |
| `mxbai-embed-large:latest` | `mixedbread-ai/mxbai-embed-large-v1` |
| `nomic-embed-text` | `nomic-ai/nomic-embed-text-v1.5` |

The model ID passed to `--chat-model` and `--embed-model` must match exactly what was loaded into the vLLM server.

## Quickstart: dual endpoint

The typical production setup runs two vLLM processes:

```bash
# Terminal 1 — embedding model
vllm serve nomic-ai/nomic-embed-text-v1.5 --port 8001

# Terminal 2 — chat model
vllm serve meta-llama/Llama-3.1-8B-Instruct --port 8000
```

```bash
# Ingest
./bin/go-to-rag --inference vllm \
  --chat-host http://localhost:8000 \
  --embed-host http://localhost:8001 \
  --embed-model nomic-ai/nomic-embed-text-v1.5 \
  ingest ./seeds

# Ask
./bin/go-to-rag --inference vllm \
  --chat-host http://localhost:8000 \
  --embed-host http://localhost:8001 \
  --embed-model nomic-ai/nomic-embed-text-v1.5 \
  --chat-model meta-llama/Llama-3.1-8B-Instruct \
  ask "What does OLM do?"
```

## API key setup

vLLM can be secured with a bearer token via `--api-key` on the server side. Pass the same token to go-to-rag:

```bash
# vllm serve ... --api-key $TOKEN
./bin/go-to-rag --inference vllm \
  --chat-host http://vllm-chat:8000 \
  --embed-host http://vllm-embed:8001 \
  --embed-model nomic-ai/nomic-embed-text-v1.5 \
  --api-key $VLLM_API_KEY \
  --chat-model meta-llama/Llama-3.1-8B-Instruct \
  ask "What does OLM do?"
```

Avoid putting credentials directly in the shell command. Use an environment variable and expand it at call time as shown above.

## All commands

Replace `<chat-url>`, `<embed-url>`, `<embed-model>`, `<chat-model>` with your values.

```bash
# ingest — only the embed endpoint is contacted
./bin/go-to-rag --inference vllm \
  --chat-host <chat-url> --embed-host <embed-url> --embed-model <embed-model> \
  ingest ./docs

# ask
./bin/go-to-rag --inference vllm \
  --chat-host <chat-url> --embed-host <embed-url> --embed-model <embed-model> \
  --chat-model <chat-model> \
  ask "your question"

# eval — only the embed endpoint is contacted
./bin/go-to-rag --inference vllm \
  --chat-host <chat-url> --embed-host <embed-url> --embed-model <embed-model> \
  eval

# mcp
./bin/go-to-rag --inference vllm \
  --chat-host <chat-url> --embed-host <embed-url> --embed-model <embed-model> \
  --chat-model <chat-model> \
  mcp

# serve (gRPC)
./bin/go-to-rag --inference vllm \
  --chat-host <chat-url> --embed-host <embed-url> --embed-model <embed-model> \
  --chat-model <chat-model> \
  serve
```

## Thinking mode

`--think` maps to vLLM's `include_reasoning` field for models that support it (Qwen3, DeepSeek-R1):

| Flag | Behaviour |
|------|-----------|
| `--think=auto` | `include_reasoning` not set; reasoning tokens routed to dim-gray output |
| `--think=hidden` | `include_reasoning` not set; reasoning tokens discarded silently |
| `--think=disabled` | `include_reasoning=false` sent to the server |

## Troubleshooting

**`returned 401`** — API key mismatch or missing. Check `--api-key` matches the token the server was started with.

**`returned 422: model not found`** — The model ID does not match what vLLM loaded. Run `curl <host>/v1/models` to see the exact ID and use that string.

**`returned 503`** — Server is starting up or the GPU ran out of memory. Check the vLLM process logs.

**Stream appears truncated with thinking models** — Large reasoning blocks can exceed the SSE scanner buffer. This is handled automatically from go-to-rag v0.1+. If you see truncated output, ensure you are running the latest build.

## Docker

`make docker-demo` targets Ollama running on the host and does not support vLLM. To use vLLM with the container binary, ensure the vLLM endpoints are reachable from within the container and pass `--network` accordingly:

```bash
docker run --rm --network host go-to-rag:latest \
  --inference vllm \
  --chat-host http://localhost:8000 \
  --embed-host http://localhost:8001 \
  --embed-model nomic-ai/nomic-embed-text-v1.5 \
  --chat-model meta-llama/Llama-3.1-8B-Instruct \
  ask "What is a CRD?"
```
