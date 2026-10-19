package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type DeleteFileInput struct {
	Path string `json:"path"`
}

type DeleteFileTool struct {
	Root string
}

func NewDeleteFileTool(root string) *DeleteFileTool {
	return &DeleteFileTool{Root: root}
}

// Mutates marks this tool as changing the workspace.
func (t *DeleteFileTool) Mutates() bool { return true }

func (t *DeleteFileTool) Name() string {
	return "delete_file"
}

func (t *DeleteFileTool) Description() string {
	return "Delete a file within the workspace"
}

func (t *DeleteFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to delete, relative to the workspace root",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
}

func (t *DeleteFileTool) Execute(ctx context.Context, input string) (string, error) {
	var args DeleteFileInput

	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&args); err != nil {
		return "", fmt.Errorf("invalid delete_file arguments: %v", err)
	}

	if err := decoder.Decode(&json.RawMessage{}); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("invalid delete_file arguments: multiple json values")
		}
		return "", fmt.Errorf("invalid delete_file arguments: %w", err)
	}

	if strings.TrimSpace(args.Path) == "" {
		return "", fmt.Errorf("path is required")
	}

	path, err := resolveInWorkspace(t.Root, args.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect file: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("delete file: %w", err)
	}

	return fmt.Sprintf("deleted %s", args.Path), nil
}
