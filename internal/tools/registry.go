package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Aneeshie/zerrcode/internal/permissions"
)

// ErrRejected is returned when the user declined a change. Nothing was written.
var ErrRejected = errors.New("the user rejected this change; nothing was written. Do not retry the same change; ask the user how they want to proceed")

// ErrNoApprover is returned when a workspace-changing tool is called but no
// approval Manager has been configured. The registry fails closed.
var ErrNoApprover = errors.New("no approval manager is configured; refusing to change the workspace")

type callIDKey struct{}

// WithCallID attaches the model's tool-call ID to ctx so approval requests can
// be correlated with the call that caused them.
func WithCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, callIDKey{}, id)
}

func callIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(callIDKey{}).(string)
	return id
}

type Registry struct {
	tools map[string]Tool

	mu       sync.RWMutex
	approver permissions.Manager
}

func NewRegistry() *Registry {
	return &Registry{
		tools: map[string]Tool{},
	}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// SetApprover configures who approves workspace-changing tool calls. With no
// approver set, such calls are refused.
func (r *Registry) SetApprover(m permissions.Manager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approver = m
}

func (r *Registry) getApprover() permissions.Manager {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.approver
}

func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, false
	}
	return tool, true
}

// Execute runs a tool on behalf of the model. It is the only path from a tool
// call to a tool, and it is where approval is enforced:
//
//   - Read-only tools run directly.
//   - A Planner tool is planned first (validation, and the real before/after
//     content), the plan is shown to the approver, and only an approved plan is
//     applied. Apply re-checks that the files are unchanged since the plan.
//   - Any other Mutator is asked for approval with its raw arguments.
//   - If approval is needed and no approver is configured, the call is refused.
func (r *Registry) Execute(ctx context.Context, name string, input string) (string, error) {
	tool, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}

	if planner, ok := tool.(Planner); ok {
		plan, err := planner.Plan(ctx, input)
		if err != nil {
			return "", err
		}

		approved, err := r.approve(ctx, permissions.Request{
			CallID:  callIDFrom(ctx),
			Tool:    name,
			Changes: plan.Changes,
		})
		if err != nil {
			return "", err
		}
		if !approved {
			return "", ErrRejected
		}
		return plan.Apply(ctx)
	}

	if r.IsMutating(name) {
		approved, err := r.approve(ctx, permissions.Request{
			CallID:  callIDFrom(ctx),
			Tool:    name,
			Details: input,
		})
		if err != nil {
			return "", err
		}
		if !approved {
			return "", ErrRejected
		}
	}

	return tool.Execute(ctx, input)
}

func (r *Registry) approve(ctx context.Context, req permissions.Request) (bool, error) {
	m := r.getApprover()
	if m == nil {
		return false, ErrNoApprover
	}
	ok, err := m.RequestApproval(ctx, req)
	if err != nil {
		return false, fmt.Errorf("approval: %w", err)
	}
	// A cancelled context must never count as approval.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	return ok, nil
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
