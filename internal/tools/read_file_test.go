package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileTool(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}

	tool := NewReadFileTool(root)

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "reads existing file", input: `{"path":"hello.txt"}`, want: "hello world"},
		{name: "missing file", input: `{"path":"nope.txt"}`, wantErr: "inspect file"},
		{name: "directory is not a file", input: `{"path":"dir"}`, wantErr: "not a regular file"},
		{name: "empty path", input: `{"path":""}`, wantErr: "path is required"},
		{name: "blank path", input: `{"path":"   "}`, wantErr: "path is required"},
		{name: "traversal", input: `{"path":"../etc/passwd"}`, wantErr: "outside the workspace"},
		{name: "deep traversal", input: `{"path":"dir/../../x"}`, wantErr: "outside the workspace"},
		{name: "invalid json", input: `{`, wantErr: "invalid read_file arguments"},
		{name: "unknown field", input: `{"path":"hello.txt","extra":1}`, wantErr: "invalid read_file arguments"},
		{name: "multiple json values", input: `{"path":"hello.txt"}{"path":"hello.txt"}`, wantErr: "invalid read_file arguments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tool.Execute(context.Background(), tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadFileToolCancelledContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := NewReadFileTool(root).Execute(ctx, `{"path":"a.txt"}`); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
