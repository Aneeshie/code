// Package permissions defines how the agent asks a human before it changes
// the workspace.
//
// The contract is enforced by tools.Registry.Execute, which is the only path
// from a model's tool call to a tool. A UI implements Manager; it cannot
// approve anything the registry did not first compute and hand to it.
package permissions

import (
	"context"

	"github.com/Aneeshie/zerrcode/internal/diff"
)

// Op is what a change does to a file.
type Op string

const (
	OpCreate Op = "create"
	OpModify Op = "modify"
	OpDelete Op = "delete"
	OpRename Op = "rename"
)

// FileChange is one file affected by a proposed operation. Diff is computed
// from the file's real current content and the content that would be written,
// never from tool arguments.
type FileChange struct {
	Path    string // workspace-relative; the new path for a rename
	OldPath string // only for renames
	Op      Op
	Diff    diff.FileDiff
}

// Request asks for approval of one tool call.
type Request struct {
	CallID  string // the model's tool-call ID, for correlating UI state
	Tool    string
	Details string // raw arguments; only used when Changes is empty
	// Changes is the exact set of changes that will be applied if approved.
	Changes []FileChange
}

// Manager decides whether a request may proceed. Returning (false, nil) means
// the user rejected it; an error means no decision could be obtained (for
// example the context was cancelled) and the operation must not proceed.
type Manager interface {
	RequestApproval(ctx context.Context, req Request) (bool, error)
}

// AllowAll approves everything. It is intended for tests and explicitly
// non-interactive use; the registry never falls back to it on its own.
type AllowAll struct{}

func (AllowAll) RequestApproval(context.Context, Request) (bool, error) { return true, nil }

// DenyAll rejects everything.
type DenyAll struct{}

func (DenyAll) RequestApproval(context.Context, Request) (bool, error) { return false, nil }
