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
	//get the tool
	tool, ok := r.Get(name)

	if !ok {
		return "", fmt.Errorf("could not fetch the toolname")
	}

	// exists so execute the tool
	res, err := tool.Execute(ctx, name)
	if err != nil {
		return "", err
	}

	return res, nil

}
