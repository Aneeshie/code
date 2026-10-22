// Package diff turns real before/after text into a structured unified diff.
//
// It exists so that anything shown to a user as "the change" is derived from
// actual file contents rather than from tool arguments or descriptions. The
// diff algorithm itself is provided by github.com/aymanbagabas/go-udiff; this
// package only classifies its output so a UI can colour and navigate it.
package diff

import (
	"fmt"
	"strings"

	"github.com/aymanbagabas/go-udiff"
)

// Kind classifies a line of a rendered diff.
type Kind int

const (
	Context Kind = iota
	Add
	Del
	Hunk // "@@ -a,b +c,d @@" header
	Meta // "\ No newline at end of file"
)

// Line is one rendered line of a diff (without a trailing newline).
type Line struct {
	Kind Kind
	Text string
}

// FileDiff is the diff of one file.
type FileDiff struct {
	Lines   []Line
	Added   int
	Removed int
	// Binary is true when either side contains a NUL byte; no lines are
	// produced in that case.
	Binary bool
}

// Empty reports whether the two sides were identical.
func (f FileDiff) Empty() bool { return !f.Binary && f.Added == 0 && f.Removed == 0 }

// Compute returns the line diff turning old into new.
func Compute(old, new string) FileDiff {
	if strings.IndexByte(old, 0) >= 0 || strings.IndexByte(new, 0) >= 0 {
		return FileDiff{Binary: old != new}
	}
	if old == new {
		return FileDiff{}
	}

	var fd FileDiff
	text := udiff.Unified("a", "b", old, new)

	for i, line := range strings.Split(text, "\n") {
		if i < 2 && (strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ")) {
			continue // file headers; callers supply their own names
		}
		switch {
		case line == "":
			// trailing newline of the whole text
		case strings.HasPrefix(line, "@@"):
			fd.Lines = append(fd.Lines, Line{Hunk, line})
		case strings.HasPrefix(line, `\`):
			fd.Lines = append(fd.Lines, Line{Meta, line})
		case line[0] == '+':
			fd.Lines = append(fd.Lines, Line{Add, line[1:]})
			fd.Added++
		case line[0] == '-':
			fd.Lines = append(fd.Lines, Line{Del, line[1:]})
			fd.Removed++
		default:
			fd.Lines = append(fd.Lines, Line{Context, strings.TrimPrefix(line, " ")})
		}
	}

	return fd
}

// Unified formats the diff as standard unified-diff text with ---/+++ headers.
// Names are used verbatim; pass "/dev/null" for the missing side of a create
// or delete.
func (f FileDiff) Unified(oldName, newName string) string {
	if f.Binary {
		return fmt.Sprintf("Binary files %s and %s differ\n", oldName, newName)
	}
	if f.Empty() {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", oldName, newName)
	for _, l := range f.Lines {
		switch l.Kind {
		case Add:
			sb.WriteString("+")
		case Del:
			sb.WriteString("-")
		case Context:
			sb.WriteString(" ")
		}
		sb.WriteString(l.Text)
		sb.WriteString("\n")
	}
	return sb.String()
}
