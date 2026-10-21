package tui

import (
	"fmt"
	"strings"

	"github.com/Aneeshie/zerrcode/internal/diff"
	"github.com/Aneeshie/zerrcode/internal/permissions"
	"github.com/charmbracelet/x/ansi"
)

// ANSI colors for a minimal, distraction-free terminal theme.
const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	dim   = "\x1b[2m"
	ital  = "\x1b[3m"
)

var (
	// Brand & Accents
	colBrand  = "\x1b[38;2;167;139;250m" // #a78bfa (Soft Violet)
	colUser   = "\x1b[38;2;56;189;248m"  // #38bdf8 (Sky Cyan)
	colTool   = "\x1b[38;2;251;191;36m"  // #fbbf24 (Warm Amber)
	colGreen  = "\x1b[38;2;52;211;153m"  // #34d399 (Emerald)
	colRed    = "\x1b[38;2;248;113;113m" // #f87171 (Soft Red)

	// Neutrals
	colBorder = "\x1b[38;2;51;65;85m"    // #334155 (Dark Slate)
	colMuted  = "\x1b[38;2;100;116;139m" // #64748b (Slate Muted)
	colText   = "\x1b[38;2;226;232;240m" // #e2e8f0 (Light Text)
)

func fg(col, text string) string {
	return col + text + reset
}

func divider(width int) string {
	if width <= 0 {
		return ""
	}
	return colBorder + strings.Repeat("─", width) + reset
}

func formatChangeSummary(c permissions.FileChange) string {
	switch c.Op {
	case permissions.OpCreate:
		return fmt.Sprintf("created %s (+%d lines)", c.Path, c.Diff.Added)
	case permissions.OpDelete:
		return fmt.Sprintf("deleted %s (-%d lines)", c.Path, c.Diff.Removed)
	case permissions.OpRename:
		return fmt.Sprintf("renamed %s -> %s (+%d -%d)", c.OldPath, c.Path, c.Diff.Added, c.Diff.Removed)
	default:
		return fmt.Sprintf("modified %s (+%d -%d)", c.Path, c.Diff.Added, c.Diff.Removed)
	}
}

func renderDiffLines(fd diff.FileDiff, maxWidth int, indent string) []string {
	if fd.Binary {
		return []string{indent + dim + fg(colTool, "[Binary file differences]")}
	}
	if fd.Empty() {
		return []string{indent + dim + fg(colMuted, "(no content changes)")}
	}

	var lines []string
	for _, l := range fd.Lines {
		truncated := l.Text
		if maxWidth > 0 && ansi.StringWidth(indent)+2+ansi.StringWidth(truncated) > maxWidth {
			truncated = ansi.Truncate(truncated, maxWidth-ansi.StringWidth(indent)-5, "...")
		}

		switch l.Kind {
		case diff.Add:
			lines = append(lines, indent+fg(colGreen, "+ "+truncated))
		case diff.Del:
			lines = append(lines, indent+fg(colRed, "- "+truncated))
		case diff.Hunk:
			lines = append(lines, indent+dim+fg(colUser, truncated))
		case diff.Meta:
			lines = append(lines, indent+dim+fg(colMuted, truncated))
		default:
			lines = append(lines, indent+dim+fg(colMuted, "  "+truncated))
		}
	}
	return lines
}

// wrapLines wraps a text block cleanly within a given maxWidth, preserving paragraphs.
func wrapLines(content string, maxWidth int, indent string) []string {
	if maxWidth <= 0 {
		maxWidth = 60
	}
	effectiveWidth := maxWidth - ansi.StringWidth(indent)
	if effectiveWidth < 10 {
		effectiveWidth = 10
	}

	var result []string
	rawLines := strings.Split(content, "\n")

	for _, rawLine := range rawLines {
		if rawLine == "" {
			result = append(result, "")
			continue
		}

		words := strings.Fields(rawLine)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		var line strings.Builder
		for _, w := range words {
			wLen := ansi.StringWidth(w)
			if line.Len() == 0 {
				line.WriteString(w)
			} else if ansi.StringWidth(line.String())+1+wLen <= effectiveWidth {
				line.WriteString(" " + w)
			} else {
				result = append(result, indent+line.String())
				line.Reset()
				line.WriteString(w)
			}
		}
		if line.Len() > 0 {
			result = append(result, indent+line.String())
		}
	}

	return result
}
