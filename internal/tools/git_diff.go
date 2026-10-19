package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxGitDiffOutput = 64 * 1024

type GitDiffInput struct {
	Path string `json:"path"`
}

// GitDiffTool is a read-only view of the working tree: the list of changed
// files plus the diff against HEAD. It exists so the agent can verify its own
// edits before claiming they are done.
type GitDiffTool struct {
	Root string
}

func NewGitDiffTool(root string) *GitDiffTool {
	return &GitDiffTool{Root: root}
}

func (t *GitDiffTool) Name() string {
	return "git_diff"
}

func (t *GitDiffTool) Description() string {
	return "Read-only. Show the files changed in the working tree (git status) and the diff against HEAD. Use it after editing to verify that exactly the intended files and hunks changed."
}

func (t *GitDiffTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Optional file or directory to limit the output to, relative to the workspace root. Defaults to the whole workspace.",
			},
		},
		"additionalProperties": false,
	}
}

func (t *GitDiffTool) Execute(ctx context.Context, input string) (string, error) {
	var args GitDiffInput

	if strings.TrimSpace(input) != "" {
		decoder := json.NewDecoder(strings.NewReader(input))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&args); err != nil {
			return "", fmt.Errorf("invalid git_diff arguments: %v", err)
		}
		if err := decoder.Decode(&json.RawMessage{}); err != io.EOF {
			return "", fmt.Errorf("invalid git_diff arguments: multiple json values")
		}
	}

	root, err := filepath.Abs(t.Root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %v", err)
	}

	var pathspec []string
	if strings.TrimSpace(args.Path) != "" {
		if err := validatePath(root, args.Path); err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}
		abs, err := resolveInWorkspace(root, args.Path)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return "", fmt.Errorf("resolve path: %v", err)
		}
		pathspec = []string{"--", rel}
	}

	if _, err := t.git(ctx, root, "rev-parse", "--is-inside-work-tree"); err != nil {
		return "", fmt.Errorf("workspace is not a git repository, cannot verify changes: %w", err)
	}

	status, err := t.git(ctx, root, append([]string{"status", "--porcelain=v1", "--untracked-files=all"}, pathspec...)...)
	if err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}

	if strings.TrimSpace(status) == "" {
		return "No changes in the working tree.", nil
	}

	diffArgs := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "HEAD"}
	diff, err := t.git(ctx, root, append(diffArgs, pathspec...)...)
	if err != nil {
		// A repository without any commit has no HEAD to diff against.
		diffArgs = []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv"}
		diff, err = t.git(ctx, root, append(diffArgs, pathspec...)...)
		if err != nil {
			return "", fmt.Errorf("git diff failed: %w", err)
		}
	}

	if len(diff) > maxGitDiffOutput {
		diff = diff[:maxGitDiffOutput] + "\n... diff truncated, pass a path to narrow it"
	}

	var out strings.Builder
	out.WriteString("Changed files (git status --porcelain):\n")
	out.WriteString(status)
	if !strings.HasSuffix(status, "\n") {
		out.WriteString("\n")
	}
	out.WriteString("\nNote: untracked files (??) are listed but not shown in the diff; use read_file to inspect them.\n")
	out.WriteString("\nDiff against HEAD:\n")
	if strings.TrimSpace(diff) == "" {
		out.WriteString("(no tracked-file changes)\n")
	} else {
		out.WriteString(diff)
	}

	return out.String(), nil
}

// git runs a read-only git command in dir. Only fixed arguments built by this
// tool are ever passed, never raw model input.
func (t *GitDiffTool) git(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"--no-pager", "--no-optional-locks", "-c", "core.quotepath=off"}, args...)

	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
		}
		return "", err
	}

	return stdout.String(), nil
}
