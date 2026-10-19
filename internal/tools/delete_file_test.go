package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteFileToolDeletesFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "gone.txt")
	mustWrite(t, path, "bye")

	out, err := NewDeleteFileTool(root).Execute(context.Background(), `{"path":"gone.txt"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "deleted") {
		t.Fatalf("output = %q", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists, stat err = %v", err)
	}
}

func TestDeleteFileToolErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "dir", "keep.txt"), "keep")

	// A file outside the workspace that a traversal must not be able to remove.
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	mustWrite(t, outside, "secret")
	rel, err := filepath.Rel(root, outside)
	if err != nil {
		t.Fatal(err)
	}

	tool := NewDeleteFileTool(root)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "missing file", input: `{"path":"nope.txt"}`, wantErr: "inspect file"},
		{name: "directory", input: `{"path":"dir"}`, wantErr: "is a directory"},
		{name: "empty path", input: `{"path":""}`, wantErr: "path is required"},
		{name: "traversal", input: `{"path":"` + filepath.ToSlash(rel) + `"}`, wantErr: "outside the workspace"},
		{name: "parent directory", input: `{"path":".."}`, wantErr: "outside the workspace"},
		{name: "invalid json", input: `{`, wantErr: "invalid delete_file arguments"},
		{name: "unknown field", input: `{"path":"a","recursive":true}`, wantErr: "invalid delete_file arguments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(context.Background(), tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}

	assertFileContent(t, outside, "secret")
	assertFileContent(t, filepath.Join(root, "dir", "keep.txt"), "keep")
}
