package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lines(ls ...string) string { return strings.Join(ls, "\n") + "\n" }

func runEdit(t *testing.T, root, patch string) (string, error) {
	t.Helper()
	in, err := json.Marshal(map[string]string{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	return NewEditFileTool(root).Execute(context.Background(), string(in))
}

func modifyPatch(name, from, to string) string {
	return lines(
		"--- a/"+name,
		"+++ b/"+name,
		"@@ -1,3 +1,3 @@",
		" one",
		"-"+from,
		"+"+to,
		" three",
	)
}

const threeLines = "one\ntwo\nthree\n"

func TestEditFileModifiesMultipleFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), threeLines)
	mustWrite(t, filepath.Join(root, "pkg", "b.txt"), threeLines)

	patch := modifyPatch("a.txt", "two", "A") + modifyPatch("pkg/b.txt", "two", "B")

	out, err := runEdit(t, root, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContent(t, filepath.Join(root, "a.txt"), "one\nA\nthree\n")
	assertFileContent(t, filepath.Join(root, "pkg", "b.txt"), "one\nB\nthree\n")
	if !strings.Contains(out, "modified a.txt") || !strings.Contains(out, "modified pkg/b.txt") {
		t.Fatalf("summary = %q", out)
	}
}

func TestEditFileCreatesDeletesAndModifiesInOnePatch(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.txt"), threeLines)
	mustWrite(t, filepath.Join(root, "old.txt"), "x\ny\n")

	patch := modifyPatch("keep.txt", "two", "2") +
		lines("--- /dev/null", "+++ b/sub/new.txt", "@@ -0,0 +1,2 @@", "+hello", "+world") +
		lines("--- a/old.txt", "+++ /dev/null", "@@ -1,2 +0,0 @@", "-x", "-y")

	if _, err := runEdit(t, root, patch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContent(t, filepath.Join(root, "keep.txt"), "one\n2\nthree\n")
	assertFileContent(t, filepath.Join(root, "sub", "new.txt"), "hello\nworld\n")
	if _, err := os.Stat(filepath.Join(root, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old.txt should be deleted, stat err = %v", err)
	}
}

func TestEditFileGitFormatAndRename(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "b", "x.txt"), threeLines)
	mustWrite(t, filepath.Join(root, "r.txt"), "same\n")

	// A real directory named "b" must not be mangled by prefix stripping.
	patch := lines(
		"diff --git a/b/x.txt b/b/x.txt",
		"--- a/b/x.txt",
		"+++ b/b/x.txt",
		"@@ -1,3 +1,3 @@",
		" one",
		"-two",
		"+2",
		" three",
	) + lines(
		"diff --git a/r.txt b/s.txt",
		"similarity index 100%",
		"rename from r.txt",
		"rename to s.txt",
	)

	out, err := runEdit(t, root, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContent(t, filepath.Join(root, "b", "x.txt"), "one\n2\nthree\n")
	assertFileContent(t, filepath.Join(root, "s.txt"), "same\n")
	if _, err := os.Stat(filepath.Join(root, "r.txt")); !os.IsNotExist(err) {
		t.Fatalf("r.txt should be gone after rename")
	}
	if !strings.Contains(out, "renamed r.txt -> s.txt") {
		t.Fatalf("summary = %q", out)
	}
}

// Every rejection below must leave the workspace untouched, even though the
// patch also contains a valid change to good.txt.
func TestEditFileRejectsBadPathsAtomically(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mustWrite(t, filepath.Join(root, "good.txt"), threeLines)
	mustWrite(t, filepath.Join(root, "other.txt"), threeLines)
	mustWrite(t, filepath.Join(root, ".git", "config"), threeLines)
	mustWrite(t, filepath.Join(outside, "secret.txt"), threeLines)
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	good := modifyPatch("good.txt", "two", "CHANGED")

	tests := []struct {
		name    string
		bad     string
		wantErr string
	}{
		{"traversal", modifyPatch("../secret.txt", "two", "x"), "'..'"},
		{"deep traversal", modifyPatch("a/../../secret.txt", "two", "x"), "'..'"},
		{"absolute path", lines("--- /dev/null", "+++ /etc/evil", "@@ -0,0 +1 @@", "+x"), "relative"},
		{"git directory", modifyPatch(".git/config", "two", "x"), ".git"},
		{"symlink escape modify", modifyPatch("escape/secret.txt", "two", "x"), "outside the workspace"},
		{"symlink escape create", lines("--- /dev/null", "+++ b/escape/new.txt", "@@ -0,0 +1 @@", "+x"), "outside the workspace"},
		{"missing file", modifyPatch("missing.txt", "two", "x"), "inspect missing.txt"},
		{"context mismatch", modifyPatch("other.txt", "NOT-THERE", "x"), "apply patch"},
		{"create over existing", lines("--- /dev/null", "+++ b/other.txt", "@@ -0,0 +1 @@", "+x"), "already exists"},
		{"same file twice", modifyPatch("good.txt", "two", "x"), "more than once"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runEdit(t, root, good+tt.bad)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}

			assertFileContent(t, filepath.Join(root, "good.txt"), threeLines)
			assertFileContent(t, filepath.Join(root, "other.txt"), threeLines)
			assertFileContent(t, filepath.Join(root, ".git", "config"), threeLines)
			assertFileContent(t, filepath.Join(outside, "secret.txt"), threeLines)
			if _, err := os.Stat(filepath.Join(outside, "new.txt")); err == nil {
				t.Fatal("file created outside the workspace")
			}
			if _, err := os.Stat(filepath.Join(root, "..", "evil")); err == nil {
				t.Fatal("file created outside the workspace")
			}
		})
	}
}

func TestEditFileInputErrors(t *testing.T) {
	root := t.TempDir()
	tool := NewEditFileTool(root)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"path field is gone", `{"path":"a.txt","patch":"x"}`, "decode edit_file input"},
		{"empty patch", `{"patch":""}`, "patch is required"},
		{"blank patch", `{"patch":"  \n"}`, "patch is required"},
		{"invalid json", `{`, "decode edit_file input"},
		{"multiple json values", `{"patch":"a"}{"patch":"b"}`, "multiple json values"},
		{"not a patch", `{"patch":"hello world"}`, "no file changes"},
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

func TestEditFileSchemaHasNoPathField(t *testing.T) {
	schema := NewEditFileTool(".").InputSchema()
	props := schema["properties"].(map[string]any)
	if _, ok := props["path"]; ok {
		t.Fatal("schema must not expose a path field")
	}
	if _, ok := props["patch"]; !ok {
		t.Fatal("schema must expose patch")
	}
}

func TestEditFileRejectsBinaryPatch(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "img.png"), "old")

	patch := "diff --git a/img.png b/img.png\nindex 1..2 100644\nBinary files a/img.png and b/img.png differ\n"

	_, err := runEdit(t, root, patch)
	if err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("error = %v, want binary rejection", err)
	}
	assertFileContent(t, filepath.Join(root, "img.png"), "old")
}
