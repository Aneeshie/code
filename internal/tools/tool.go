package tools

import "context"

type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(ctx context.Context, input string) (string, error)
}

// Mutator is an optional interface for tools that change the workspace. The
// agent uses it to know when changes need to be verified before the work is
// reported as complete.
type Mutator interface {
	Mutates() bool
}
