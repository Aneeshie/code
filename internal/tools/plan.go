package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Aneeshie/zerrcode/internal/diff"
	"github.com/Aneeshie/zerrcode/internal/permissions"
)

// Plan is a fully validated, not-yet-applied operation. Changes describe
// exactly what Apply will write, computed from the files' real current content,
// so a UI can show a faithful diff before anything touches the disk.
type Plan struct {
	Changes []permissions.FileChange

	// guard re-checks that the workspace still matches what the plan was
	// computed from. It runs immediately before commit.
	guard  func() error
	commit func() (string, error)
}

// Apply writes the planned changes. It refuses to write if any affected file
// changed after the plan was made (for example while waiting for approval), so
// edits made by the user in the meantime are never overwritten.
func (p *Plan) Apply(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	if p.guard != nil {
		if err := p.guard(); err != nil {
			return "", err
		}
	}
	return p.commit()
}

// Planner is implemented by tools that change the workspace. Plan does all
// validation and computes the result without writing anything. Execute on such
// a tool is Plan followed by Apply, with no approval in between; the approval
// gate lives in Registry.Execute, which is the only entry point used by the
// agent.
type Planner interface {
	Plan(ctx context.Context, input string) (*Plan, error)
}

// fileState is a snapshot of a file used to detect concurrent modification.
type fileState struct {
	path    string
	exists  bool
	content []byte
}

func snapshot(path string) (fileState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileState{path: path}, nil
		}
		return fileState{}, err
	}
	return fileState{path: path, exists: true, content: data}, nil
}

// verify returns an error if the file no longer matches the snapshot.
func (s fileState) verify() error {
	cur, err := snapshot(s.path)
	if err != nil {
		return fmt.Errorf("re-check %s: %w", filepath.Base(s.path), err)
	}
	if cur.exists != s.exists || !bytes.Equal(cur.content, s.content) {
		return fmt.Errorf("%s changed after the change was proposed; nothing was written. Re-read it and propose the change again", filepath.Base(s.path))
	}
	return nil
}

func verifyAll(states []fileState) func() error {
	return func() error {
		for _, s := range states {
			if err := s.verify(); err != nil {
				return err
			}
		}
		return nil
	}
}

func change(op permissions.Op, path, oldPath, before, after string) permissions.FileChange {
	return permissions.FileChange{
		Path:    path,
		OldPath: oldPath,
		Op:      op,
		Diff:    diff.Compute(before, after),
	}
}
