package tools

import (
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
