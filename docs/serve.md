# serve

Start a gRPC server that exposes the RAG pipeline over three RPCs: `Ask`, `RetrieveChunks`, and `GetServerConfig`.

```bash
./bin/go-to-rag serve
```

## Flags

| Flag               | Default                    | Description                                                                                                                       |
|--------------------|----------------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| `--grpc-addr`      | `:50051`                   | Listen address                                                                                                                    |
| `--top-k`          | `10`                       | Default number of chunks to retrieve per request; individual requests may override via `top_k`                                    |
| `--model`          | `llama3.2:1b`              | Ollama chat model (used by `Ask`)                                                                                                 |
| `--with-fallback`  | `false`                    | Allow the model to answer from its own knowledge when context is missing                                                          |
| `--host`           | `http://localhost:11434`   | Ollama host URL                                                                                                                   |
| `--embed-model`    | `mxbai-embed-large:latest` | Embedding model                                                                                                                   |
| `--db`             | `./data/index.db`          | Vector store database path                                                                                                        |

## Thinking modes

The server supports three thinking token modes controlled **per-request** via the `think_mode` field on `AskRequest`. The server is stateless — it does not enforce a server-level default. When a client omits `think_mode`, the server uses `THINK_MODE_AUTO` (the model's native default).

| Mode       | `think_mode` proto value   | Behaviour                                                                                                              |
|------------|---------------------------|------------------------------------------------------------------------------------------------------------------------|
| Auto       | `THINK_MODE_AUTO` (0)     | Model uses its default — for qwen3 this means chain-of-thought tokens are streamed back in `AskResponse.thinking`      |
| Disabled   | `THINK_MODE_DISABLED` (1) | Model skips reasoning entirely, lowering latency when chain-of-thought is not needed                                   |
| Hidden     | `THINK_MODE_HIDDEN` (2)   | Model reasons internally but thinking tokens are discarded before reaching the stream; caller only sees `answer` chunks |

## RPCs

For local testing, install the required tools:

```bash
# gRPC CLI (for all RPCs)
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# gRPC health check probe (for Health Check RPC)
go install github.com/grpc-ecosystem/grpc-health-probe@latest
```

### Ask (server-streaming)

Retrieves the top-k chunks, injects them as context, and streams the LLM response token by token.

**Request fields:**
- `question` (string, required)
- `top_k` (int32, optional — overrides server default when > 0)
- `think_mode` (ThinkMode enum, optional — overrides server `--think` default for this request)

**Response** streams `oneof content { answer, thinking }` — each message carries either an answer token or a thinking token. Clients can:
- Render both: show thinking in a different style, then the final answer
- Strip thinking: only collect `answer` chunks and ignore `thinking`
- Log thinking: record reasoning for observability

```bash
# Basic question using server defaults
grpcurl -plaintext \
  -d '{"question": "What does OLM do?", "top_k": 5}' \
  localhost:50051 rag.v1.RAGService/Ask

# Override think mode to auto for this request (thinking tokens streamed)
grpcurl -plaintext \
  -d '{"question": "What does OLM do?", "think_mode": "THINK_MODE_AUTO"}' \
  localhost:50051 rag.v1.RAGService/Ask

# Override think mode to disabled — skip reasoning, lower latency
grpcurl -plaintext \
  -d '{"question": "What does OLM do?", "think_mode": "THINK_MODE_DISABLED"}' \
  localhost:50051 rag.v1.RAGService/Ask

# Override think mode to hidden — model reasons but caller sees answer only
grpcurl -plaintext \
  -d '{"question": "What does OLM do?", "think_mode": "THINK_MODE_HIDDEN"}' \
  localhost:50051 rag.v1.RAGService/Ask
```

### RetrieveChunks (unary)

Returns scored chunks without running generation. Useful for service-to-service retrieval where the caller handles its own LLM step.

**Request fields:** `question` (string), `top_k` (int32).
**Response:** list of chunks with `text`, `source`, `score`, and `chunk_index`.

```bash
grpcurl -plaintext \
  -d '{"question": "What is a CRD?", "top_k": 3}' \
  localhost:50051 rag.v1.RAGService/RetrieveChunks
```

### GetServerConfig (unary)

Returns the server's configuration. Currently always returns `THINK_MODE_AUTO` since the server is stateless and does not enforce a server-level thinking mode.

**Response fields:**
- `default_think_mode` (ThinkMode enum) — always `THINK_MODE_AUTO`; clients should specify `think_mode` on `Ask` requests to override

```bash
grpcurl -plaintext \
  -d '{}' \
  localhost:50051 rag.v1.RAGService/GetServerConfig
```

Response:

```json
{
  "defaultThinkMode": "THINK_MODE_AUTO"
}
```

## Health Check

The server registers the standard gRPC health check service. Use `grpc_health_probe` to verify:

```bash
# overall server health
grpc-health-probe -addr :50051

# service-specific check
grpc-health-probe -addr :50051 -service rag.v1.RAGService
```

Both return exit code `0` and print `status: SERVING` when the server is ready.

## Service definition

See [`proto/rag/v1/rag.proto`](../proto/rag/v1/rag.proto). To regenerate Go stubs after editing the proto:

```bash
make proto
```

See [docs/protobuf.md](protobuf.md) for the full codegen workflow.