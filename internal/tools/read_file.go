package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type ReadFileInput struct {
	Path string `json:"path"`
}

type ReadFileTool struct {
	Root string
}

func NewReadFileTool(root string) *ReadFileTool {
	return &ReadFileTool{
		Root: root,
	}
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read the contents of the file within the workspace"
}

func (t *ReadFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read, relative to the workspace root",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, input string) (string, error) {

	var args ReadFileInput

	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&args); err != nil {
		return "", fmt.Errorf("invalid read_file arguments: %v", err)
	}

	if err := decoder.Decode(&json.RawMessage{}); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("invalid read_file arguments: multiple json values")
		}
		return "", fmt.Errorf("invalid read_file arguments: %w", err)
	}

	if strings.TrimSpace(args.Path) == "" {
		return "", fmt.Errorf("path is required")
	}

	path, err := resolveInWorkspace(t.Root, args.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("inspect file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path is not a regular file")
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	return string(data), nil

}
