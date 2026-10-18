package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolveInWorkspace joins p onto root and ensures the result stays inside root.
func resolveInWorkspace(root, p string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %v", err)
	}

	path, err := filepath.Abs(filepath.Join(absRoot, p))
	if err != nil {
		return "", fmt.Errorf("resolve path: %v", err)
	}

	rel, err := filepath.Rel(absRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside the workspace")
	}

	return path, nil
}
