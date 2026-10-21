package diff

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

func TestComputeCountsAndKinds(t *testing.T) {
	fd := Compute("one\ntwo\nthree\n", "one\nTWO\nthree\nfour\n")

	if fd.Added != 2 || fd.Removed != 1 {
		t.Fatalf("added/removed = %d/%d, want 2/1\n%+v", fd.Added, fd.Removed, fd.Lines)
	}
	if fd.Lines[0].Kind != Hunk {
		t.Fatalf("first line should be a hunk header: %+v", fd.Lines[0])
	}

	var del, add []string
	for _, l := range fd.Lines {
		switch l.Kind {
		case Del:
			del = append(del, l.Text)
		case Add:
			add = append(add, l.Text)
		}
	}
	if strings.Join(del, ",") != "two" || strings.Join(add, ",") != "TWO,four" {
		t.Fatalf("del=%v add=%v", del, add)
	}
}

func TestComputeIdenticalAndBinary(t *testing.T) {
	if fd := Compute("same\n", "same\n"); !fd.Empty() || fd.Unified("a", "b") != "" {
		t.Fatalf("identical input must produce an empty diff: %+v", fd)
	}

	fd := Compute("text", "bin\x00ary")
	if !fd.Binary || len(fd.Lines) != 0 {
		t.Fatalf("expected binary diff, got %+v", fd)
	}
	if !strings.Contains(fd.Unified("a", "b"), "Binary files") {
		t.Fatalf("binary unified output = %q", fd.Unified("a", "b"))
	}
}

func TestComputeCreateAndDelete(t *testing.T) {
	create := Compute("", "a\nb\n")
	if create.Added != 2 || create.Removed != 0 {
		t.Fatalf("create: %+v", create)
	}
	del := Compute("a\nb\n", "")
	if del.Added != 0 || del.Removed != 2 {
		t.Fatalf("delete: %+v", del)
	}
}

func TestComputeNoNewlineAtEndOfFile(t *testing.T) {
	fd := Compute("a\nb", "a\nb\n")
	if fd.Empty() {
		t.Fatal("a trailing-newline change must show up in the diff")
	}
	found := false
	for _, l := range fd.Lines {
		if l.Kind == Meta {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a no-newline marker: %+v", fd.Lines)
	}
}

// The diff must be a faithful description of the change: applying its unified
// text to the old content with an independent implementation (go-gitdiff) has
// to reproduce the new content exactly.
func TestUnifiedRoundTripsThroughGitdiff(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	words := []string{"alpha", "beta", "gamma", "delta", "", "  indented", "tab\tsep"}

	gen := func() string {
		n := rng.Intn(30)
		var sb strings.Builder
		for i := 0; i < n; i++ {
			sb.WriteString(words[rng.Intn(len(words))])
			if i < n-1 || rng.Intn(4) != 0 {
				sb.WriteString("\n")
			}
		}
		return sb.String()
	}

	for i := 0; i < 300; i++ {
		old, new := gen(), gen()
		fd := Compute(old, new)
		if old == new {
			if !fd.Empty() {
				t.Fatalf("case %d: identical input produced a diff", i)
			}
			continue
		}

		patch := "diff --git a/f b/f\n" + fd.Unified("a/f", "b/f")
		files, _, err := gitdiff.Parse(strings.NewReader(patch))
		if err != nil || len(files) != 1 {
			t.Fatalf("case %d: parse: %v (%d files)\n%s", i, err, len(files), patch)
		}

		var out bytes.Buffer
		if err := gitdiff.Apply(&out, bytes.NewReader([]byte(old)), files[0]); err != nil {
			t.Fatalf("case %d: apply: %v\nold=%q\nnew=%q\n%s", i, err, old, new, patch)
		}
		if out.String() != new {
			t.Fatalf("case %d: round trip mismatch\nold=%q\nnew=%q\ngot=%q", i, old, new, out.String())
		}
	}
}
