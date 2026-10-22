package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Aneeshie/zerrcode/internal/permissions"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

const maxPatchFiles = 50

// patchRetryGuidance tells the model what to do when a hunk does not apply.
// The failure usually means the file differs from what the model remembers,
// and the tempting shortcut is to overwrite the whole file, which silently
// discards anything the patch did not know about.
func patchRetryGuidance(oldName, newName string, kind editKind) string {
	target := newName
	if kind != editCreate && oldName != "" {
		target = oldName
	}
	return fmt.Sprintf(
		"No files were modified. Do not fall back to write_file or rewrite the whole file. "+
			"Re-read %q with read_file, then generate a new, minimal patch with fresh context lines copied from its current content.",
		target,
	)
}

type EditFileInput struct {
	Patch string `json:"patch"`
}

type EditFileTool struct {
	Root string `json:"root"`
}

func NewEditFileTool(root string) *EditFileTool {
	return &EditFileTool{Root: root}
}

// Mutates marks this tool as changing the workspace.
func (t *EditFileTool) Mutates() bool { return true }

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Description() string {
	return "Apply a unified diff (git format) to the workspace. A single patch may modify, create, delete or rename several files; the files are identified by the patch itself. " +
		"The patch is all-or-nothing: if any hunk fails to apply, no file is changed. " +
		"If it fails, re-read the file with read_file and send a new, targeted patch; do not fall back to rewriting the whole file with write_file."
}

func (t *EditFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"patch": map[string]any{
				"type":        "string",
				"description": "A unified diff. Paths in the headers are relative to the workspace root. Use /dev/null for created or deleted files.",
			},
		},
		"required":             []string{"patch"},
		"additionalProperties": false,
	}
}

type editKind int

const (
	editModify editKind = iota
	editCreate
	editDelete
	editRename
)

// plannedEdit is a fully validated and computed change, ready to be written.
type plannedEdit struct {
	kind    editKind
	name    string // display name
	oldPath string // absolute; empty for creates
	newPath string // absolute; empty for deletes
	content []byte
	before  []byte // content before the edit; empty for creates
	relOld  string // workspace-relative old path
	relNew  string // workspace-relative new path
	mode    os.FileMode
}

// fileChange describes the edit for display and approval. The diff is computed
// from the real before and after contents.
func (e plannedEdit) fileChange() permissions.FileChange {
	switch e.kind {
	case editCreate:
		return change(permissions.OpCreate, e.relNew, "", "", string(e.content))
	case editDelete:
		return change(permissions.OpDelete, e.relOld, "", string(e.before), "")
	case editRename:
		return change(permissions.OpRename, e.relNew, e.relOld, string(e.before), string(e.content))
	default:
		return change(permissions.OpModify, e.relNew, "", string(e.before), string(e.content))
	}
}

func (t *EditFileTool) Execute(ctx context.Context, input string) (string, error) {
	plan, err := t.Plan(ctx, input)
	if err != nil {
		return "", err
	}
	return plan.Apply(ctx)
}

// Plan validates the patch and computes every result in memory without writing.
func (t *EditFileTool) Plan(ctx context.Context, input string) (*Plan, error) {
	var inputs EditFileInput

	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inputs); err != nil {
		return nil, fmt.Errorf("decode edit_file input: %w", err)
	}
	if err := decoder.Decode(&json.RawMessage{}); err != io.EOF {
		return nil, fmt.Errorf("decode edit_file input: multiple json values")
	}

	if strings.TrimSpace(inputs.Patch) == "" {
		return nil, fmt.Errorf("patch is required")
	}

	if strings.TrimSpace(t.Root) == "" {
		return nil, fmt.Errorf("root is required")
	}

	files, _, err := gitdiff.Parse(strings.NewReader(inputs.Patch))
	if err != nil {
		return nil, fmt.Errorf("parse patch: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("patch contains no file changes")
	}
	if len(files) > maxPatchFiles {
		return nil, fmt.Errorf("patch touches %d files, max is %d", len(files), maxPatchFiles)
	}

	// Phase 1: validate every file and compute every result in memory. Nothing
	// is written unless the whole patch is valid and applies cleanly.
	gitFormat := strings.HasPrefix(inputs.Patch, "diff --git ") || strings.Contains(inputs.Patch, "\ndiff --git ")
	seen := map[string]bool{}
	var edits []plannedEdit

	for i, file := range files {
		edit, err := t.planEdit(file, gitFormat, seen)
		if err != nil {
			return nil, fmt.Errorf("file %d of %d: %w", i+1, len(files), err)
		}
		edits = append(edits, edit)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// The plan is applied in phase 2, after approval. Guards ensure no
	// affected file changed in between.
	var states []fileState
	var changes []permissions.FileChange
	for _, e := range edits {
		changes = append(changes, e.fileChange())
		if e.oldPath != "" {
			states = append(states, fileState{path: e.oldPath, exists: true, content: e.before})
		}
		if e.kind == editCreate || e.kind == editRename {
			states = append(states, fileState{path: e.newPath})
		}
	}

	return &Plan{
		Changes: changes,
		guard:   verifyAll(states),
		commit: func() (string, error) {
			var summary []string
			for i, edit := range edits {
				if err := commitEdit(edit); err != nil {
					applied := ""
					if i > 0 {
						applied = fmt.Sprintf(" (%d earlier file(s) were already written: %s)", i, strings.Join(summary, ", "))
					}
					return "", fmt.Errorf("write %s: %w%s", edit.name, err, applied)
				}
				summary = append(summary, edit.describe())
			}
			return strings.Join(summary, "\n"), nil
		},
	}, nil
}

func (e plannedEdit) describe() string {
	switch e.kind {
	case editCreate:
		return "created " + e.name
	case editDelete:
		return "deleted " + e.name
	case editRename:
		return "renamed " + e.name
	default:
		return "modified " + e.name
	}
}

func (t *EditFileTool) planEdit(file *gitdiff.File, gitFormat bool, seen map[string]bool) (plannedEdit, error) {
	if file.IsBinary {
		return plannedEdit{}, fmt.Errorf("binary patches are not supported")
	}
	if file.IsCopy {
		return plannedEdit{}, fmt.Errorf("copy patches are not supported")
	}

	// gitdiff leaves the old name empty for new files and the new name empty
	// for deletions, so only validate the side that exists.
	var oldName, newName string
	var oldPath, newPath string

	if !file.IsNew {
		oldName = stripDiffPrefix(file.OldName, gitFormat)
		if err := validatePath(t.Root, oldName); err != nil {
			return plannedEdit{}, fmt.Errorf("invalid old path %q: %w", file.OldName, err)
		}
		p, err := resolveInWorkspace(t.Root, oldName)
		if err != nil {
			return plannedEdit{}, fmt.Errorf("invalid old path %q: %w", file.OldName, err)
		}
		oldPath = p
	}

	if !file.IsDelete {
		newName = stripDiffPrefix(file.NewName, gitFormat)
		if err := validatePath(t.Root, newName); err != nil {
			return plannedEdit{}, fmt.Errorf("invalid new path %q: %w", file.NewName, err)
		}
		p, err := resolveInWorkspace(t.Root, newName)
		if err != nil {
			return plannedEdit{}, fmt.Errorf("invalid new path %q: %w", file.NewName, err)
		}
		newPath = p
	}

	edit := plannedEdit{oldPath: oldPath, newPath: newPath, relOld: oldName, relNew: newName}

	switch {
	case file.IsNew:
		edit.kind, edit.name = editCreate, newName
	case file.IsDelete:
		edit.kind, edit.name = editDelete, oldName
	case file.IsRename:
		edit.kind, edit.name = editRename, oldName+" -> "+newName
	default:
		if oldPath != newPath {
			return plannedEdit{}, fmt.Errorf("old path %q and new path %q differ but patch is not a rename", oldName, newName)
		}
		edit.kind, edit.name = editModify, newName
	}

	// A path may appear only once in a patch: later entries would otherwise be
	// applied against content that no longer matches what was validated.
	keys := []string{}
	for _, p := range []string{oldPath, newPath} {
		if p != "" && (len(keys) == 0 || keys[0] != p) {
			keys = append(keys, p)
		}
	}
	for _, p := range keys {
		if seen[p] {
			return plannedEdit{}, fmt.Errorf("%s appears more than once in the patch", edit.name)
		}
	}
	for _, p := range keys {
		seen[p] = true
	}

	// Read the original content.
	var original []byte
	if edit.kind != editCreate {
		info, err := os.Stat(oldPath)
		if err != nil {
			return plannedEdit{}, fmt.Errorf("inspect %s: %w", oldName, err)
		}
		if !info.Mode().IsRegular() {
			return plannedEdit{}, fmt.Errorf("%s is not a regular file", oldName)
		}
		edit.mode = info.Mode().Perm()

		original, err = os.ReadFile(oldPath)
		if err != nil {
			return plannedEdit{}, fmt.Errorf("read %s: %w", oldName, err)
		}
	}

	// Creates and renames must not clobber an existing file.
	if edit.kind == editCreate || edit.kind == editRename {
		if _, err := os.Lstat(newPath); err == nil {
			return plannedEdit{}, fmt.Errorf("%s already exists", newName)
		} else if !os.IsNotExist(err) {
			return plannedEdit{}, fmt.Errorf("inspect %s: %w", newName, err)
		}
	}

	if edit.kind == editCreate {
		edit.mode = 0o644
		if m := file.NewMode.Perm(); m != 0 {
			edit.mode = m
		}
	}

	var output bytes.Buffer
	if err := gitdiff.Apply(&output, bytes.NewReader(original), file); err != nil {
		return plannedEdit{}, fmt.Errorf("apply patch to %s: %w. %s", edit.name, err, patchRetryGuidance(oldName, newName, edit.kind))
	}
	edit.content = output.Bytes()
	edit.before = original

	return edit, nil
}

func commitEdit(e plannedEdit) error {
	switch e.kind {
	case editDelete:
		return os.Remove(e.oldPath)

	case editCreate, editRename:
		if err := os.MkdirAll(filepath.Dir(e.newPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(e.newPath, e.content, e.mode); err != nil {
			return err
		}
		if e.kind == editRename {
			return os.Remove(e.oldPath)
		}
		return nil

	default:
		return os.WriteFile(e.newPath, e.content, e.mode)
	}
}

// stripDiffPrefix removes the conventional a/ or b/ prefix from a header
// name. gitdiff already strips it for headers inside a "diff --git" block, so
// it is only removed for plain unified diffs. For plain diffs gitdiff reports
// the same (prefixed) name on both sides of a modification, so either prefix
// is accepted on either side, and at most one is removed.
func stripDiffPrefix(name string, gitFormat bool) string {
	if gitFormat || name == "/dev/null" {
		return name
	}
	if strings.HasPrefix(name, "a/") {
		return strings.TrimPrefix(name, "a/")
	}
	return strings.TrimPrefix(name, "b/")
}

const maxPathLength = 4096

// validatePath checks a path taken from a patch before it touches the
// filesystem. It must be a non-empty, relative path with no NUL/control
// characters, no ".." segments, and no .git component, and it must resolve
// (including through symlinks) to somewhere inside the workspace. "/dev/null"
// is allowed as the placeholder for the missing side of a created or deleted
// file.
func validatePath(root, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is required")
	}

	if path == "/dev/null" {
		return nil
	}

	if len(path) > maxPathLength {
		return fmt.Errorf("path is too long (%d bytes, max %d)", len(path), maxPathLength)
	}

	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("path contains control characters")
		}
	}

	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) || filepath.VolumeName(path) != "" {
		return fmt.Errorf("path must be relative to the workspace")
	}

	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		switch part {
		case "..":
			return fmt.Errorf("path must not contain '..' segments")
		case ".git":
			return fmt.Errorf("access to .git is not allowed")
		}
	}

	if _, err := resolveInWorkspace(root, path); err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	return nil
}
