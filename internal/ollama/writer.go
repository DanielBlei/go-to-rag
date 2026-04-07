package ollama

import "io"

// ThinkingWriter is an optional extension of io.Writer that accepts thinking
// tokens on a separate channel. Transport-specific writers (e.g. gRPC streamWriter)
// implement this to route thinking to a dedicated proto field.
type ThinkingWriter interface {
	io.Writer
	WriteThinking(p []byte) (int, error)
}
