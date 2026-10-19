package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "simple", path: "main.go"},
		{name: "nested", path: "pkg/util/util.go"},
		{name: "dev null placeholder", path: "/dev/null"},
		{name: "dots in names", path: "..foo/.hidden/x..y"},
		{name: "gitignore is fine", path: ".gitignore"},
		{name: "empty", path: "", wantErr: "path is required"},
		{name: "blank", path: "   ", wantErr: "path is required"},
		{name: "absolute", path: "/etc/passwd", wantErr: "relative"},
		{name: "windows style absolute", path: `\windows\system32`, wantErr: "relative"},
		{name: "parent", path: "../x", wantErr: "'..'"},
		{name: "inner parent", path: "a/../b", wantErr: "'..'"},
		{name: "backslash parent", path: `a\..\..\x`, wantErr: "'..'"},
		{name: "git dir", path: ".git/config", wantErr: ".git"},
		{name: "nested git dir", path: "sub/.git/hooks/pre-commit", wantErr: ".git"},
		{name: "nul byte", path: "a\x00b", wantErr: "control characters"},
		{name: "newline", path: "a\nb", wantErr: "control characters"},
		{name: "too long", path: strings.Repeat("a", maxPathLength+1), wantErr: "too long"},
		{name: "symlink escape", path: "escape/file.txt", wantErr: "outside the workspace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(root, tt.path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestStripDiffPrefix(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		gitFormat bool
		want      string
	}{
		{"plain a prefix", "a/x.go", false, "x.go"},
		{"plain b prefix", "b/x.go", false, "x.go"},
		{"plain strips only one prefix", "a/b/x.go", false, "b/x.go"},
		{"plain without prefix", "x.go", false, "x.go"},
		{"git format already stripped", "b/x.go", true, "b/x.go"},
		{"dev null untouched", "/dev/null", false, "/dev/null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripDiffPrefix(tt.in, tt.gitFormat); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
