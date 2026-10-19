package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type WriteFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type WriteFileTool struct {
	Root string
}

func NewWriteFileTool(root string) *WriteFileTool {
	return &WriteFileTool{Root: root}
}

// Mutates marks this tool as changing the workspace.
func (t *WriteFileTool) Mutates() bool { return true }

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Create a file, or overwrite an existing file, with the given content"
}

func (t *WriteFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write, relative to the workspace root",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required":             []string{"path", "content"},
		"additionalProperties": false,
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, input string) (string, error) {
	var args WriteFileInput

	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&args); err != nil {
		return "", fmt.Errorf("invalid write_file arguments: %v", err)
	}

	if err := decoder.Decode(&json.RawMessage{}); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("invalid write_file arguments: multiple json values")
		}
		return "", fmt.Errorf("invalid write_file arguments: %w", err)
	}

	if strings.TrimSpace(args.Path) == "" {
		return "", fmt.Errorf("path is required")
	}

	path, err := resolveInWorkspace(t.Root, args.Path)
	if err != nil {
		return "", err
	}

	perm := os.FileMode(0o644)
	existed := false
	if info, err := os.Stat(path); err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("path is not a regular file")
		}
		perm = info.Mode().Perm()
		existed = true
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect file: %w", err)
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create parent directories: %w", err)
	}

	if err := os.WriteFile(path, []byte(args.Content), perm); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	action := "created"
	if existed {
		action = "updated"
	}
	return fmt.Sprintf("%s %s (%d bytes)", action, args.Path, len(args.Content)), nil
}
