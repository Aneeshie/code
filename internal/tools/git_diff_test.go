package tools

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	mustWrite(t, filepath.Join(root, "a.txt"), "one\ntwo\n")
	mustWrite(t, filepath.Join(root, "pkg", "b.txt"), "b\n")
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return root
}

func TestGitDiffToolNoChanges(t *testing.T) {
	root := newGitRepo(t)

	out, err := NewGitDiffTool(root).Execute(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "No changes in the working tree." {
		t.Fatalf("output = %q", out)
	}
}

func TestGitDiffToolListsChangedFilesAndDiff(t *testing.T) {
	root := newGitRepo(t)
	mustWrite(t, filepath.Join(root, "a.txt"), "one\nTWO\n")
	mustWrite(t, filepath.Join(root, "brand", "new.txt"), "new\n")

	for _, input := range []string{``, `{}`, `{"path":""}`} {
		out, err := NewGitDiffTool(root).Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", input, err)
		}

		for _, want := range []string{" M a.txt", "?? brand/new.txt", "-two", "+TWO", "untracked"} {
			if !strings.Contains(out, want) {
				t.Errorf("input %q: output missing %q:\n%s", input, want, out)
			}
		}
		if strings.Contains(out, "pkg/b.txt") {
			t.Errorf("unchanged file listed:\n%s", out)
		}
	}
}

func TestGitDiffToolPathFilter(t *testing.T) {
	root := newGitRepo(t)
	mustWrite(t, filepath.Join(root, "a.txt"), "changed\n")
	mustWrite(t, filepath.Join(root, "pkg", "b.txt"), "changed\n")

	out, err := NewGitDiffTool(root).Execute(context.Background(), `{"path":"pkg"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "pkg/b.txt") || strings.Contains(out, "a.txt") {
		t.Fatalf("path filter not applied:\n%s", out)
	}
}

func TestGitDiffToolDoesNotModifyWorkingTree(t *testing.T) {
	root := newGitRepo(t)
	mustWrite(t, filepath.Join(root, "a.txt"), "changed\n")

	if _, err := NewGitDiffTool(root).Execute(context.Background(), `{}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, filepath.Join(root, "a.txt"), "changed\n")
}

func TestGitDiffToolTruncatesLargeDiff(t *testing.T) {
	root := newGitRepo(t)
	mustWrite(t, filepath.Join(root, "a.txt"), strings.Repeat("a long changed line\n", maxGitDiffOutput))

	out, err := NewGitDiffTool(root).Execute(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "truncated") {
		t.Fatal("expected truncation notice")
	}
}

func TestGitDiffToolErrors(t *testing.T) {
	root := newGitRepo(t)
	tool := NewGitDiffTool(root)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"traversal", `{"path":"../"}`, "'..'"},
		{"absolute", `{"path":"/etc"}`, "relative"},
		{"git dir", `{"path":".git"}`, ".git"},
		{"unknown field", `{"staged":true}`, "invalid git_diff arguments"},
		{"invalid json", `{`, "invalid git_diff arguments"},
		{"multiple values", `{}{}`, "multiple json values"},
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

func TestGitDiffToolOutsideRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	_, err := NewGitDiffTool(t.TempDir()).Execute(context.Background(), `{}`)
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("error = %v, want not a git repository", err)
	}
}
