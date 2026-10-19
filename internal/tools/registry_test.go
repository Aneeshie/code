package tools

import "testing"

func TestMutatingToolsAreMarked(t *testing.T) {
	reg := NewRegistry()
	for _, tool := range []Tool{
		NewReadFileTool("."), NewListFilesTool("."), NewSearchCodeTool("."), NewGitDiffTool("."),
		NewWriteFileTool("."), NewDeleteFileTool("."), NewEditFileTool("."),
	} {
		reg.Register(tool)
	}

	for name, want := range map[string]bool{
		"read_file": false, "list_files": false, "search_code": false, "git_diff": false,
		"write_file": true, "delete_file": true, "edit_file": true,
		"unknown": false,
	} {
		if got := reg.IsMutating(name); got != want {
			t.Errorf("IsMutating(%q) = %v, want %v", name, got, want)
		}
	}
}
