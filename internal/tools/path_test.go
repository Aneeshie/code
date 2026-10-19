package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveInWorkspace(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "simple file", path: "a.txt"},
		{name: "nested file", path: "a/b/c.txt"},
		{name: "empty path is the root", path: ""},
		{name: "dot is the root", path: "."},
		{name: "dot dot inside workspace", path: "a/../b.txt"},
		{name: "name that merely starts with dots", path: "..foo"},
		{name: "absolute path is contained in root", path: "/etc/passwd"},
		{name: "parent directory", path: "..", wantErr: true},
		{name: "parent file", path: "../secret.txt", wantErr: true},
		{name: "deep traversal", path: "a/../../secret.txt", wantErr: true},
		{name: "many parents", path: "../../../../../../etc/passwd", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveInWorkspace(root, tt.path)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got path %q", got)
				}
				if !strings.Contains(err.Error(), "outside the workspace") {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			absRoot, _ := filepath.Abs(root)
			if got != absRoot && !strings.HasPrefix(got, absRoot+string(filepath.Separator)) {
				t.Fatalf("path %q escaped workspace %q", got, absRoot)
			}
		})
	}
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
}

func TestResolveInWorkspaceSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret.txt"), "secret")
	mustWrite(t, filepath.Join(root, "real", "ok.txt"), "ok")

	symlinkOrSkip(t, outside, filepath.Join(root, "escape"))
	symlinkOrSkip(t, filepath.Join(outside, "secret.txt"), filepath.Join(root, "secret-link.txt"))
	symlinkOrSkip(t, filepath.Join(root, "real"), filepath.Join(root, "internal-link"))

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "symlinked dir escaping workspace", path: "escape", wantErr: true},
		{name: "file through escaping dir symlink", path: "escape/secret.txt", wantErr: true},
		{name: "new file through escaping dir symlink", path: "escape/new.txt", wantErr: true},
		{name: "file symlink escaping workspace", path: "secret-link.txt", wantErr: true},
		{name: "symlink staying inside workspace", path: "internal-link/ok.txt"},
		{name: "new file under internal symlink", path: "internal-link/new.txt"},
		{name: "new file in new dir", path: "brand/new/file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveInWorkspace(root, tt.path)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "outside the workspace") {
					t.Fatalf("error = %v, want outside the workspace", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestToolsRejectSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	mustWrite(t, secret, "secret")
	symlinkOrSkip(t, outside, filepath.Join(root, "escape"))
	symlinkOrSkip(t, secret, filepath.Join(root, "secret-link.txt"))

	ctx := context.Background()
	cases := []struct {
		name string
		tool Tool
		in   string
	}{
		{"read_file", NewReadFileTool(root), `{"path":"escape/secret.txt"}`},
		{"read_file link", NewReadFileTool(root), `{"path":"secret-link.txt"}`},
		{"list_files", NewListFilesTool(root), `{"path":"escape"}`},
		{"write_file", NewWriteFileTool(root), `{"path":"escape/evil.txt","content":"x"}`},
		{"write_file link", NewWriteFileTool(root), `{"path":"secret-link.txt","content":"pwned"}`},
		{"delete_file", NewDeleteFileTool(root), `{"path":"escape/secret.txt"}`},
		{"delete_file link", NewDeleteFileTool(root), `{"path":"secret-link.txt"}`},
		{"search_code", NewSearchCodeTool(root), `{"query":"secret","path":"escape"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.tool.Execute(ctx, c.in)
			if err == nil || !strings.Contains(err.Error(), "outside the workspace") {
				t.Fatalf("error = %v, want outside the workspace", err)
			}
		})
	}

	assertFileContent(t, secret, "secret")
	if _, err := os.Stat(filepath.Join(outside, "evil.txt")); err == nil {
		t.Fatal("file was written outside the workspace through a symlink")
	}
}
