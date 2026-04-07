package ollama

import (
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// ThinkingWriter is an optional extension of io.Writer that accepts thinking tokens on a separate channel.
// Transport-specific writers (e.g. gRPC streamWriter) implement this to route thinking to a dedicated proto field.
type ThinkingWriter interface {
	io.Writer
	WriteThinking(p []byte) (int, error)
}

// TerminalWriter wraps any io.Writer and implements ThinkingWriter.
// Answers are forwarded unchanged via Write. Thinking tokens are written
// in ANSI dark-gray when the underlying writer is a TTY; otherwise raw text.
type TerminalWriter struct {
	w     io.Writer
	isTTY bool
}

// NewTerminalWriter returns a TerminalWriter that writes to w.
// It detects if w is a TTY for styling thinking tokens.
func NewTerminalWriter(w io.Writer) *TerminalWriter {
	isTTY := false
	if f, ok := w.(*os.File); ok {
		isTTY = isatty.IsTerminal(f.Fd())
	}
	return &TerminalWriter{w: w, isTTY: isTTY}
}

// Write forwards p to the underlying writer unchanged.
func (t *TerminalWriter) Write(p []byte) (int, error) {
	return t.w.Write(p)
}

// WriteThinking writes p to the underlying writer in ANSI dark-gray
// if it's a TTY; otherwise writes raw text.
func (t *TerminalWriter) WriteThinking(p []byte) (int, error) {
	if t.isTTY {
		_, err := fmt.Fprintf(t.w, "\033[90m%s\033[0m", p)
		if err != nil {
			return 0, err
		}
		return len(p), nil
	}
	return t.w.Write(p)
}
