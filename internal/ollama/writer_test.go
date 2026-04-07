package ollama

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTerminalWriter_Write(t *testing.T) {
	// Test that Write forwards bytes unchanged.
	var buf bytes.Buffer
	tw := NewTerminalWriter(&buf)

	n, err := tw.Write([]byte("test content"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 12 {
		t.Errorf("Write returned %d, want 12", n)
	}
	if buf.String() != "test content" {
		t.Errorf("got %q, want 'test content'", buf.String())
	}
}

func TestTerminalWriter_WriteThinking_NonTTY(t *testing.T) {
	// Test that WriteThinking writes raw text when not a TTY.
	var buf bytes.Buffer
	tw := NewTerminalWriter(&buf)

	n, err := tw.WriteThinking([]byte("thinking..."))
	if err != nil {
		t.Fatalf("WriteThinking failed: %v", err)
	}
	if n != 11 {
		t.Errorf("WriteThinking returned %d, want 11", n)
	}

	output := buf.String()
	if output != "thinking..." {
		t.Errorf("got %q, want 'thinking...'", output)
	}
	// Ensure no ANSI codes when not a TTY.
	if strings.Contains(output, "\033[90m") {
		t.Errorf("unexpected ANSI codes in non-TTY output: %q", output)
	}
}

func TestTerminalWriter_WriteThinking_TTY(t *testing.T) {
	// Test that WriteThinking wraps in ANSI gray when isTTY is true.
	// We can't easily test with a real TTY, so we'll manually set isTTY.
	var buf bytes.Buffer
	tw := &TerminalWriter{
		w:     &buf,
		isTTY: true,
	}

	n, err := tw.WriteThinking([]byte("thinking..."))
	if err != nil {
		t.Fatalf("WriteThinking failed: %v", err)
	}
	if n != 11 {
		t.Errorf("WriteThinking returned %d, want 11", n)
	}

	output := buf.String()
	// Verify ANSI gray codes are present.
	if !strings.Contains(output, "\033[90m") {
		t.Errorf("expected ANSI code in TTY output, got: %q", output)
	}
	if !strings.Contains(output, "\033[0m") {
		t.Errorf("expected ANSI reset in TTY output, got: %q", output)
	}
	// Verify content is there.
	if !strings.Contains(output, "thinking...") {
		t.Errorf("expected content in output, got: %q", output)
	}
}

func TestNewTerminalWriter_DetectsTTY(t *testing.T) {
	// Test that NewTerminalWriter correctly detects TTY for os.Stdout.
	tw := NewTerminalWriter(os.Stdout)

	// We can't really assert the TTY detection without mocking,
	// but we can verify the TerminalWriter was created.
	if tw == nil {
		t.Fatal("NewTerminalWriter returned nil")
	}

	// For non-File writers, isTTY should be false.
	var buf bytes.Buffer
	tw2 := NewTerminalWriter(&buf)
	if tw2.isTTY {
		t.Error("expected isTTY=false for bytes.Buffer")
	}
}

func TestTerminalWriter_ImplementsThinkingWriter(t *testing.T) {
	// Verify that TerminalWriter implements ThinkingWriter interface.
	var tw ThinkingWriter
	var buf bytes.Buffer
	tw = NewTerminalWriter(&buf)

	// If this compiles, the interface is satisfied.
	if tw == nil {
		t.Fatal("TerminalWriter should satisfy ThinkingWriter interface")
	}
}
