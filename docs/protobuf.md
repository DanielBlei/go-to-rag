# Protobuf

The gRPC API is defined in `proto/rag/v1/rag.proto`. Generated Go code lives in `internal/gen/rag/v1/` and is checked into the repository so `go build` works without codegen tools.

## Prerequisites

Install `buf` and the Go protoc plugins:

```bash
# buf CLI
# macOS
brew install bufbuild/buf/buf
# or from source
go install github.com/bufbuild/buf/cmd/buf@latest

# Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Make sure `$GOPATH/bin` (or `$GOBIN`) is on your `$PATH`.

## Regenerating

After editing `.proto` files, regenerate the Go code:

```bash
make proto
```

This runs `buf generate`, which reads `buf.gen.yaml` and outputs to `internal/gen/`. Always commit the regenerated files alongside your proto changes.

## Linting

Proto lint runs as part of `make lint`. To run it standalone:

```bash
buf lint
```

Lint rules are configured in `buf.yaml` using the `STANDARD` rule set.

## AskResponse oneof

The `Ask` RPC streams `oneof content { answer, thinking }`. This allows clients to distinguish between answer tokens and thinking tokens in the stream:

```protobuf
message AskResponse {
  oneof content {
    string answer   = 1;
    string thinking = 2;
  }
}
```

Clients receive each chunk tagged as either an answer or thinking token. Use `msg.GetAnswer()` or `msg.GetThinking()` to read the appropriate field. Only one of these fields is set per message.

**Thinking tokens are only streamed when:**
- The model supports thinking (e.g. `qwen3:*`)
- `--think=auto` or no `--think` flag is set on the server
- `--show-thinking=true` (default)

## Adding new messages or RPCs

1. Edit `proto/rag/v1/rag.proto`
2. Run `make proto` to regenerate
3. Run `make lint` to validate both Go and proto
4. Implement the new RPC in `internal/grpcserver/`
5. Commit the proto change, generated code, and implementation together

**Note on breaking changes:** The `AskResponse` proto was changed from a flat `string answer = 1` to a `oneof content`. This is a breaking wire change — existing compiled clients must be recompiled to work with the new schema. Update clients to use `msg.GetAnswer()` and handle `GetThinking()` for the new field.
