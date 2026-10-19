package tools

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newSearchFixture(t *testing.T) *SearchCodeTool {
	t.Helper()
	if _, err := exec.LookPath("grep"); err != nil {
		t.Skip("grep not available")
	}

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.go"), "package main\nfunc Hello() {}\n")
	mustWrite(t, filepath.Join(root, "notes.txt"), "Hello from notes\n")
	mustWrite(t, filepath.Join(root, "pkg", "util.go"), "package pkg\n// Hello helper\n")
	mustWrite(t, filepath.Join(root, ".git", "config"), "Hello in git metadata\n")
	mustWrite(t, filepath.Join(root, "regex.txt"), "a.b and a+b\n")
	mustWrite(t, filepath.Join(root, "dash.txt"), "--version flag\n")

	return NewSearchCodeTool(root)
}

func TestSearchCodeToolFindsMatches(t *testing.T) {
	tool := newSearchFixture(t)

	out, err := tool.Execute(context.Background(), `{"query":"Hello"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"main.go:2:", "notes.txt:1:", filepath.Join("pkg", "util.go") + ":2:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, ".git") {
		t.Errorf(".git should be excluded:\n%s", out)
	}
}

func TestSearchCodeToolFilePattern(t *testing.T) {
	tool := newSearchFixture(t)

	out, err := tool.Execute(context.Background(), `{"query":"Hello","file_pattern":"*.go"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "main.go") || strings.Contains(out, "notes.txt") {
		t.Fatalf("file_pattern not applied:\n%s", out)
	}
}

func TestSearchCodeToolSubPath(t *testing.T) {
	tool := newSearchFixture(t)

	out, err := tool.Execute(context.Background(), `{"query":"Hello","path":"pkg"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "util.go") || strings.Contains(out, "main.go") {
		t.Fatalf("path not applied:\n%s", out)
	}
}

func TestSearchCodeToolNoMatchesIsNotAnError(t *testing.T) {
	tool := newSearchFixture(t)

	out, err := tool.Execute(context.Background(), `{"query":"definitely-not-present"}`)
	if err != nil {
		t.Fatalf("no matches should not be an error: %v", err)
	}
	if out != "No matches found" {
		t.Fatalf("output = %q", out)
	}
}

func TestSearchCodeToolQueryIsLiteral(t *testing.T) {
	tool := newSearchFixture(t)

	out, err := tool.Execute(context.Background(), `{"query":"a.b"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "regex.txt") {
		t.Fatalf("expected literal match:\n%s", out)
	}

	out, err = tool.Execute(context.Background(), `{"query":"a+b"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "regex.txt") {
		t.Fatalf("expected literal '+' match:\n%s", out)
	}
}

func TestSearchCodeToolQueryStartingWithDash(t *testing.T) {
	tool := newSearchFixture(t)

	out, err := tool.Execute(context.Background(), `{"query":"--version"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "dash.txt") {
		t.Fatalf("dash query treated as a flag:\n%s", out)
	}
}

func TestSearchCodeToolErrors(t *testing.T) {
	tool := newSearchFixture(t)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "empty query", input: `{"query":""}`, wantErr: "query is required"},
		{name: "blank query", input: `{"query":"   "}`, wantErr: "query is required"},
		{name: "missing query", input: `{}`, wantErr: "query is required"},
		{name: "traversal", input: `{"query":"x","path":"../"}`, wantErr: "outside the workspace"},
		{name: "deep traversal", input: `{"query":"x","path":"pkg/../../"}`, wantErr: "outside the workspace"},
		{name: "missing path", input: `{"query":"x","path":"nope"}`, wantErr: "grep failed"},
		{name: "invalid json", input: `{`, wantErr: "invalid search_code arguments"},
		{name: "unknown field", input: `{"query":"x","regex":true}`, wantErr: "invalid search_code arguments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(context.Background(), tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSearchCodeToolTruncatesLargeOutput(t *testing.T) {
	tool := newSearchFixture(t)
	mustWrite(t, filepath.Join(tool.Root, "big.txt"), strings.Repeat("needle line\n", maxSearchOutput))

	out, err := tool.Execute(context.Background(), `{"query":"needle","path":"big.txt"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "truncated") {
		t.Fatal("expected truncation notice")
	}
	if len(out) > maxSearchOutput+200 {
		t.Fatalf("output too large: %d bytes", len(out))
	}
}
