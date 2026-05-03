package rag

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTerminalWriter_Write(t *testing.T) {
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
	if strings.Contains(output, "\033[90m") {
		t.Errorf("unexpected ANSI codes in non-TTY output: %q", output)
	}
}

func TestTerminalWriter_WriteThinking_TTY(t *testing.T) {
	var buf bytes.Buffer
	tw := &TerminalWriter{w: &buf, isTTY: true}

	n, err := tw.WriteThinking([]byte("thinking..."))
	if err != nil {
		t.Fatalf("WriteThinking failed: %v", err)
	}
	if n != 11 {
		t.Errorf("WriteThinking returned %d, want 11", n)
	}

	output := buf.String()
	if !strings.Contains(output, "\033[90m") {
		t.Errorf("expected ANSI code in TTY output, got: %q", output)
	}
	if !strings.Contains(output, "\033[0m") {
		t.Errorf("expected ANSI reset in TTY output, got: %q", output)
	}
	if !strings.Contains(output, "thinking...") {
		t.Errorf("expected content in output, got: %q", output)
	}
}

func TestNewTerminalWriter_DetectsTTY(t *testing.T) {
	tw := NewTerminalWriter(os.Stdout)
	if tw == nil {
		t.Fatal("NewTerminalWriter returned nil")
	}

	var buf bytes.Buffer
	tw2 := NewTerminalWriter(&buf)
	if tw2.isTTY {
		t.Error("expected isTTY=false for bytes.Buffer")
	}
}

func TestTerminalWriter_ImplementsThinkingWriter(t *testing.T) {
	var tw ThinkingWriter
	var buf bytes.Buffer
	tw = NewTerminalWriter(&buf)
	_ = tw
}
