package tools

import "context"

type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(ctx context.Context, input string) (string, error)
}
