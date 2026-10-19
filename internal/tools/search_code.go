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

const maxSearchOutput = 32 * 1024

type SearchCodeInputs struct {
	Query       string `json:"query"`
	Path        string `json:"path"`
	FilePattern string `json:"file_pattern"`
}

type SearchCodeTool struct {
	Root string
}

func NewSearchCodeTool(root string) *SearchCodeTool {
	return &SearchCodeTool{Root: root}
}

func (s *SearchCodeTool) Name() string {
	return "search_code"
}

func (s *SearchCodeTool) Description() string {
	return "Search for code in the repository"
}

func (s *SearchCodeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Literal text to search for (not a regex)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory or file to search, relative to the workspace root. Defaults to the root.",
			},
			"file_pattern": map[string]any{
				"type":        "string",
				"description": "Glob of file names to include, e.g. *.go",
			},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func (s *SearchCodeTool) Execute(ctx context.Context, input string) (string, error) {
	var inputs SearchCodeInputs

	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inputs); err != nil {
		return "", fmt.Errorf("invalid search_code arguments: %v", err)
	}
	if err := decoder.Decode(&json.RawMessage{}); err != io.EOF {
		return "", fmt.Errorf("invalid search_code arguments: multiple json values")
	}

	if strings.TrimSpace(inputs.Query) == "" {
		return "", fmt.Errorf("query is required")
	}

	absPath, err := resolveInWorkspace(s.Root, inputs.Path)
	if err != nil {
		return "", err
	}

	root, err := filepath.Abs(s.Root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %v", err)
	}

	// Run from the workspace root and use a relative path so results are
	// short and relative to the workspace.
	relPath, err := filepath.Rel(root, absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %v", err)
	}

	args := []string{"-rnIF", "--exclude-dir=.git"}
	if inputs.FilePattern != "" {
		args = append(args, "--include="+inputs.FilePattern)
	}
	args = append(args, "-e", inputs.Query, "--", relPath)

	cmd := exec.CommandContext(ctx, "grep", args...)
	cmd.Dir = root

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		// grep exits with 1 when nothing matched.
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "No matches found", nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("grep failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	output := stdout.String()
	if len(output) > maxSearchOutput {
		output = output[:maxSearchOutput] + "\n... output truncated, narrow your search"
	}

	return output, nil
}
