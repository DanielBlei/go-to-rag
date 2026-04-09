package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	ragv1 "github.com/DanielBlei/go-to-rag/internal/gen/rag/v1"
	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

const (
	testEmbedModel = "mxbai-embed-large:latest"
	testChatModel  = "llama3.2:1b"
)

func TestServe_RetrieveChunks(t *testing.T) {
	// 1. Real SQLite db in temp dir, seeded with one chunk
	dbFile := filepath.Join(t.TempDir(), "test.db")
	store, err := vectorstore.NewSQLite(dbFile)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	if err := store.AddChunk(
		context.Background(),
		"doc.md",
		"pods are the smallest unit",
		[]float32{1, 0, 0, 0},
		0,
	); err != nil {
		t.Fatalf("AddChunk: %v", err)
	}
	_ = store.Close()

	// 2. Fake Ollama server — handles /api/tags and /api/embed
	ollamaServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/tags":
				models := []api.ListModelResponse{
					{Model: testEmbedModel},
					{Model: testChatModel},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(api.ListResponse{Models: models})
			case "/api/embed":
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(api.EmbedResponse{
					Model:      "x:latest",
					Embeddings: [][]float32{{1, 0, 0, 0}},
				})
			default:
				http.NotFound(w, r)
			}
		}),
	)
	defer ollamaServer.Close()

	// 3. Pre-bind listener, held open until runServe calls ServeListener,
	// eliminating the listen-close-rebind race.
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()

	// 4. Set package-level vars used by runServe
	host = ollamaServer.URL
	embedModel = testEmbedModel
	serveModel = testChatModel
	dbPath = dbFile
	serveTopK = 5
	serveWithFallback = false
	grpcListener = lis
	defer func() { grpcListener = nil }()

	// 5. Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	serveErr := make(chan error, 1)
	go func() { serveErr <- runServe(cmd, nil) }()

	// 6. Wait for server to be ready (retry for up to 2 seconds)
	conn, err := waitForServer(t, addr, 2*time.Second)
	if err != nil {
		t.Fatalf("server did not start: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// 7. Make a RetrieveChunks request
	client := ragv1.NewRAGServiceClient(conn)
	resp, err := client.RetrieveChunks(context.Background(), &ragv1.RetrieveChunksRequest{
		Question: "what are pods?",
		TopK:     5,
	})
	if err != nil {
		t.Fatalf("RetrieveChunks: %v", err)
	}
	if len(resp.GetChunks()) == 0 {
		t.Error("expected at least one chunk, got none")
	}

	// 8. Shut down and verify clean exit
	cancel()
	select {
	case err := <-serveErr:
		if err != nil && status.Code(err) != codes.Canceled {
			t.Errorf("serve exited with unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("server did not shut down in time")
	}
}

// waitForServer dials the gRPC server in a retry loop until it responds or timeout elapses.
func waitForServer(t *testing.T, addr string, timeout time.Duration) (*grpc.ClientConn, error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			// Probe with a real call; server may be listening but not yet serving
			cl := ragv1.NewRAGServiceClient(conn)
			_, callErr := cl.RetrieveChunks(
				context.Background(),
				&ragv1.RetrieveChunksRequest{Question: "probe", TopK: 1},
			)
			if callErr == nil || status.Code(callErr) != codes.Unavailable {
				return conn, nil
			}
			_ = conn.Close()
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out waiting for gRPC server at %s", addr)
}
