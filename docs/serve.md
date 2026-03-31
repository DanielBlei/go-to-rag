# serve

Start a gRPC server that exposes the RAG pipeline over two RPCs.

```bash
./bin/go-to-rag serve
```

## Flags

| Flag              | Default                   | Description                                                              |
|-------------------|---------------------------|--------------------------------------------------------------------------|
| `--grpc-addr`     | `:50051`                  | Listen address                                                           |
| `--top-k`         | `10`                      | Default number of chunks to retrieve per request                         |
| `--model`         | `llama3.2:1b`             | Ollama chat model (used by `Ask`)                                        |
| `--with-fallback` | `false`                   | Allow the model to answer from its own knowledge when context is missing |
| `--host`          | `http://localhost:11434`  | Ollama host URL                                                          |
| `--embed-model`   | `nomic-embed-text:latest` | Embedding model                                                          |
| `--db`            | `./data/index.db`         | Vector store database path                                               |

## RPCs

For local testing, install the required tools:

```bash
# gRPC CLI (for Ask and RetrieveChunks RPCs)
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# gRPC health check probe (for Health Check RPC)
go install github.com/grpc-ecosystem/grpc-health-probe@latest
```

### Ask (server-streaming)

Retrieves the top-k chunks, injects them as context, and streams the LLM response token by token.

Request fields: `question` (string), `top_k` (int32, overrides server default if > 0).

```bash
grpcurl -plaintext \
  -import-path . -proto proto/rag/v1/rag.proto \
  -d '{"question": "What does OLM do?", "top_k": 5}' \
  localhost:50051 rag.v1.RAGService/Ask
```

### RetrieveChunks (unary)

Returns scored chunks without running generation. Useful for service-to-service retrieval where the caller handles its own LLM step.

Request fields: `question` (string), `top_k` (int32). Response: list of chunks with `text`, `source`, `score`, and `chunk_index`.

```bash
grpcurl -plaintext \
  -d '{"question": "What is a CRD?", "top_k": 3}' \
  localhost:50051 rag.v1.RAGService/RetrieveChunks
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
