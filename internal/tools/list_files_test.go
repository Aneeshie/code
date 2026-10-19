package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListFilesTool(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "b.txt"), "b")
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, "sub", "inner.txt"), "i")
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	tool := NewListFilesTool(root)

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "root via empty path", input: `{"path":""}`, want: "a.txt\nb.txt\nempty/\nsub/"},
		{name: "root via omitted path", input: `{}`, want: "a.txt\nb.txt\nempty/\nsub/"},
		{name: "root via empty input", input: ``, want: "a.txt\nb.txt\nempty/\nsub/"},
		{name: "subdirectory", input: `{"path":"sub"}`, want: "inner.txt"},
		{name: "empty directory", input: `{"path":"empty"}`, want: ""},
		{name: "missing directory", input: `{"path":"nope"}`, wantErr: "inspect directory"},
		{name: "file is not a directory", input: `{"path":"a.txt"}`, wantErr: "not a directory"},
		{name: "traversal", input: `{"path":".."}`, wantErr: "outside the workspace"},
		{name: "deep traversal", input: `{"path":"sub/../../"}`, wantErr: "outside the workspace"},
		{name: "invalid json", input: `{`, wantErr: "invalid list_files arguments"},
		{name: "unknown field", input: `{"dir":"sub"}`, wantErr: "invalid list_files arguments"},
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

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
