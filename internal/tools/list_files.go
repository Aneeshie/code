package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ListFilesInput struct {
	Path string `json:"path"`
}

type ListFilesTool struct {
	Root string
}

func NewListFilesTool(root string) *ListFilesTool {
	return &ListFilesTool{Root: root}
}

func (t *ListFilesTool) Name() string {
	return "list_files"
}

func (t *ListFilesTool) Description() string {
	return "List all files in the given directory"
}

func (t *ListFilesTool) Execute(ctx context.Context, input string) (string, error) {
	var args ListFilesInput

	if strings.TrimSpace(input) != "" {
		decoder := json.NewDecoder(strings.NewReader(input))
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&args); err != nil {
			return "", fmt.Errorf("invalid list_files arguments: %v", err)
		}

		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			if err == nil {
				return "", fmt.Errorf("invalid list_files arguments: multiple json values")
			}
			return "", fmt.Errorf("invalid list_files arguments: %w", err)
		}
	}

	root, err := filepath.Abs(t.Root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %v", err)
	}

	path, err := filepath.Abs(filepath.Join(root, args.Path))
	if err != nil {
		return "", fmt.Errorf("resolve directory path: %v", err)
	}

	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside the workspace")
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("inspect directory: %w", err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("read directory: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)

	return strings.Join(names, "\n"), nil
}
