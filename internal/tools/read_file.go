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

func (t *ReadFileTool) Execute(ctx context.Context, input string) (string, error) {

	var args ReadFileInput

	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&args); err != nil {
		return "", fmt.Errorf("invalid read_file arguments: %v", err)
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("invalid read_file arguments: multiple json values")
		}
		return "", fmt.Errorf("invalid read_file arguments: %w", err)
	}

	if strings.TrimSpace(args.Path) == "" {
		return "", fmt.Errorf("path is required")
	}

	root, err := filepath.Abs(t.Root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %v", err)
	}

	path := filepath.Join(root, args.Path)

	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve file path: %v", err)
	}

	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside the workspace")
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
