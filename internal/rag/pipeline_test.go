package rag

import (
	"context"
	"errors"
	"testing"

	"github.com/DanielBlei/go-to-rag/internal/vectorstore"
)

// fakeClient implements Embedder for testing.
type fakeClient struct {
	vec []float32
	err error
}

func (f *fakeClient) Embed(_ context.Context, _ string) ([]float32, error) {
	return f.vec, f.err
}

// fakeStore implements vectorstore.Store for testing.
type fakeStore struct {
	results []vectorstore.Result
	err     error
}

func (f *fakeStore) Search(_ context.Context, _ []float32, _ int) ([]vectorstore.Result, error) {
	return f.results, f.err
}
func (f *fakeStore) AddChunk(_ context.Context, _, _ string, _ []float32, _ int) error { return nil }
func (f *fakeStore) CountChunks(_ context.Context) (int, error)                        { return 0, nil }
func (f *fakeStore) HasSource(_ context.Context, _ string) (bool, error)               { return false, nil }
func (f *fakeStore) DeleteSource(_ context.Context, _ string) error                    { return nil }
func (f *fakeStore) Close() error                                                      { return nil }

// Compile-time interface validation
var _ Embedder = (*fakeClient)(nil)
var _ Pipeline = (*pipeline)(nil)

func TestRetrieve(t *testing.T) {
	embedErr := errors.New("embed failed")
	searchErr := errors.New("search failed")

	tests := []struct {
		name        string
		client      *fakeClient
		store       *fakeStore
		wantContext string
		wantErr     error
	}{
		{
			name:   "returns joined chunks",
			client: &fakeClient{vec: []float32{1, 0, 0}},
			store: &fakeStore{results: []vectorstore.Result{
				{Text: "chunk one"},
				{Text: "chunk two"},
				{Text: "chunk three"},
			}},
			wantContext: "chunk one\n---\nchunk two\n---\nchunk three",
		},
		{
			name:        "empty store returns empty string",
			client:      &fakeClient{vec: []float32{1, 0, 0}},
			store:       &fakeStore{results: nil},
			wantContext: "",
		},
		{
			name:    "embed error is propagated",
			client:  &fakeClient{err: embedErr},
			store:   &fakeStore{},
			wantErr: embedErr,
		},
		{
			name:    "search error is propagated",
			client:  &fakeClient{vec: []float32{1, 0, 0}},
			store:   &fakeStore{err: searchErr},
			wantErr: searchErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Retrieve(context.Background(), "query", 5, tt.client, tt.store)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantContext {
				t.Errorf("context = %q, want %q", got, tt.wantContext)
			}
		})
	}
}

func TestRetrieveChunks(t *testing.T) {
	embedErr := errors.New("embed failed")
	searchErr := errors.New("search failed")

	tests := []struct {
		name    string
		client  *fakeClient
		store   *fakeStore
		want    []vectorstore.Result
		wantErr error
	}{
		{
			name:   "returns structured results",
			client: &fakeClient{vec: []float32{1, 0, 0}},
			store: &fakeStore{results: []vectorstore.Result{
				{Source: "k8s.md", Text: "chunk one", ChunkIndex: 0, Score: 0.95},
				{Source: "k8s.md", Text: "chunk two", ChunkIndex: 1, Score: 0.80},
			}},
			want: []vectorstore.Result{
				{Source: "k8s.md", Text: "chunk one", ChunkIndex: 0, Score: 0.95},
				{Source: "k8s.md", Text: "chunk two", ChunkIndex: 1, Score: 0.80},
			},
		},
		{
			name:   "empty store returns nil",
			client: &fakeClient{vec: []float32{1, 0, 0}},
			store:  &fakeStore{results: nil},
			want:   nil,
		},
		{
			name:    "embed error is propagated",
			client:  &fakeClient{err: embedErr},
			store:   &fakeStore{},
			wantErr: embedErr,
		},
		{
			name:    "search error is propagated",
			client:  &fakeClient{vec: []float32{1, 0, 0}},
			store:   &fakeStore{err: searchErr},
			wantErr: searchErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RetrieveChunks(context.Background(), "query", 5, tt.client, tt.store)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d results, want %d", len(got), len(tt.want))
			}
			for i, r := range got {
				if r.Source != tt.want[i].Source {
					t.Errorf("result[%d].Source = %q, want %q", i, r.Source, tt.want[i].Source)
				}
				if r.Text != tt.want[i].Text {
					t.Errorf("result[%d].Text = %q, want %q", i, r.Text, tt.want[i].Text)
				}
				if r.ChunkIndex != tt.want[i].ChunkIndex {
					t.Errorf("result[%d].ChunkIndex = %d, want %d", i, r.ChunkIndex, tt.want[i].ChunkIndex)
				}
				if r.Score != tt.want[i].Score {
					t.Errorf("result[%d].Score = %f, want %f", i, r.Score, tt.want[i].Score)
				}
			}
		})
	}
}

func TestPipeline_Retrieve(t *testing.T) {
	store := &fakeStore{results: []vectorstore.Result{
		{Text: "chunk one"},
		{Text: "chunk two"},
	}}
	r := NewPipeline(&fakeClient{vec: []float32{1, 0, 0}}, store)
	got, err := r.Retrieve(context.Background(), "query", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "chunk one\n---\nchunk two"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
