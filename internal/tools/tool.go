package tools

import "context"

type Tool interface {
	Name() string
	Description() string

	Execute(ctx context.Context, input string) (string, error)
}
