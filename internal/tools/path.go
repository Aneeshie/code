package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var errOutsideWorkspace = fmt.Errorf("path is outside the workspace")

// resolveInWorkspace joins p onto root and ensures the result stays inside
// root, both lexically (no ".." escapes) and after following symlinks.
func resolveInWorkspace(root, p string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %v", err)
	}

	path, err := filepath.Abs(filepath.Join(absRoot, p))
	if err != nil {
		return "", fmt.Errorf("resolve path: %v", err)
	}

	if !within(absRoot, path) {
		return "", errOutsideWorkspace
	}

	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %v", err)
	}

	realPath, err := evalExisting(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %v", err)
	}

	if !within(realRoot, realPath) {
		return "", errOutsideWorkspace
	}

	return path, nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// evalExisting resolves symlinks in the longest existing prefix of path and
// re-appends the not-yet-existing remainder, so it also works for files that
// are about to be created.
func evalExisting(path string) (string, error) {
	var rest []string
	current := path

	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, rest[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		rest = append(rest, filepath.Base(current))
		current = parent
	}
}
