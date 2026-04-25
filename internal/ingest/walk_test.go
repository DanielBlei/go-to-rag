package ingest

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWalkFiles(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	hidden := filepath.Join(dir, ".hidden")
	for _, d := range []string{sub, hidden} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeFile := func(path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rootMD := filepath.Join(dir, "root.md")
	rootTXT := filepath.Join(dir, "root.txt")
	hiddenMD := filepath.Join(dir, ".hidden.md")
	subMD := filepath.Join(sub, "sub.md")
	hiddenDirMD := filepath.Join(hidden, "inside.md")
	writeFile(rootMD)
	writeFile(rootTXT)
	writeFile(hiddenMD)
	writeFile(subMD)
	writeFile(hiddenDirMD)

	tests := []struct {
		name          string
		root          string
		glob          string
		noRecursive   bool
		includeHidden bool
		want          []string
		wantErr       bool
	}{
		{
			name: "recurses by default",
			root: dir, glob: "*.md",
			want: []string{rootMD, subMD},
		},
		{
			name: "no-recursive finds only root files",
			root: dir, glob: "*.md", noRecursive: true,
			want: []string{rootMD},
		},
		{
			name: "hidden dir skipped by default",
			root: dir, glob: "*.md",
			want: []string{rootMD, subMD},
		},
		{
			name: "include-hidden includes hidden dirs and files",
			root: dir, glob: "*.md", includeHidden: true,
			want: []string{hiddenMD, hiddenDirMD, rootMD, subMD},
		},
		{
			name: "hidden file skipped by default",
			root: dir, glob: "*.md", noRecursive: true,
			want: []string{rootMD},
		},
		{
			name: "glob filters by extension",
			root: dir, glob: "*.txt",
			want: []string{rootTXT},
		},
		{
			name: "no matches returns empty slice",
			root: dir, glob: "*.go",
			want: nil,
		},
		{
			name:    "nonexistent root returns error",
			root:    filepath.Join(dir, "missing"),
			glob:    "*.md",
			wantErr: true,
		},
		{
			name:    "invalid glob pattern returns error",
			root:    dir,
			glob:    "[bad",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := walkFiles(tt.root, tt.glob, tt.noRecursive, tt.includeHidden)
			if (err != nil) != tt.wantErr {
				t.Fatalf("walkFiles() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			slices.Sort(got)
			if len(got) != len(tt.want) {
				t.Fatalf("walkFiles() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("walkFiles()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
