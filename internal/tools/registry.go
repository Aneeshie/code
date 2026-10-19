package tools

import (
	"context"
	"fmt"
)

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: map[string]Tool{},
	}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, false
	}
	return tool, true
}

func (r *Registry) Execute(ctx context.Context, name string, input string) (string, error) {
	tool, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}

	return tool.Execute(ctx, input)
}

func (r *Registry) All() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// IsMutating reports whether the named tool changes the workspace.
func (r *Registry) IsMutating(name string) bool {
	tool, ok := r.Get(name)
	if !ok {
		return false
	}
	m, ok := tool.(Mutator)
	return ok && m.Mutates()
}
