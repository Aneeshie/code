package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileToolCreatesFile(t *testing.T) {
	root := t.TempDir()
	tool := NewWriteFileTool(root)

	out, err := tool.Execute(context.Background(), `{"path":"new.txt","content":"hello"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "created") {
		t.Fatalf("output = %q, want created prefix", out)
	}

	assertFileContent(t, filepath.Join(root, "new.txt"), "hello")
}

func TestWriteFileToolUpdatesExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "exec.sh")
	mustWrite(t, path, "old content that is longer")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := NewWriteFileTool(root).Execute(context.Background(), `{"path":"exec.sh","content":"new"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "updated") {
		t.Fatalf("output = %q, want updated prefix", out)
	}

	assertFileContent(t, path, "new")

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("permissions = %v, want 0755 preserved", info.Mode().Perm())
	}
}

func TestWriteFileToolCreatesParentDirectories(t *testing.T) {
	root := t.TempDir()

	if _, err := NewWriteFileTool(root).Execute(context.Background(), `{"path":"a/b/c.txt","content":"x"}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContent(t, filepath.Join(root, "a", "b", "c.txt"), "x")
}

func TestWriteFileToolAllowsEmptyContent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "f.txt"), "something")

	if _, err := NewWriteFileTool(root).Execute(context.Background(), `{"path":"f.txt","content":""}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContent(t, filepath.Join(root, "f.txt"), "")
}

func TestWriteFileToolErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	tool := NewWriteFileTool(root)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "empty path", input: `{"path":"","content":"x"}`, wantErr: "path is required"},
		{name: "traversal", input: `{"path":"../evil.txt","content":"x"}`, wantErr: "outside the workspace"},
		{name: "deep traversal", input: `{"path":"a/../../evil.txt","content":"x"}`, wantErr: "outside the workspace"},
		{name: "directory target", input: `{"path":"dir","content":"x"}`, wantErr: "not a regular file"},
		{name: "invalid json", input: `{`, wantErr: "invalid write_file arguments"},
		{name: "unknown field", input: `{"path":"a","content":"x","mode":"0777"}`, wantErr: "invalid write_file arguments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(context.Background(), tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}

	// A rejected traversal must not create anything outside the workspace.
	outside := filepath.Join(filepath.Dir(root), "evil.txt")
	if _, err := os.Stat(outside); err == nil {
		t.Fatalf("file was written outside the workspace: %s", outside)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("content of %s = %q, want %q", path, got, want)
	}
}
