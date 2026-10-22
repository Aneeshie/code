package tui

import (
	"context"
	"fmt"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/Aneeshie/zerrcode/internal/permissions"
)

// approvalPromptMsg is sent to the Bubble Tea program when a mutating tool needs user approval.
type approvalPromptMsg struct {
	req    permissions.Request
	respCh chan bool
}

// TUIApprover implements permissions.Manager by delegating approval prompts to the Bubble Tea TUI.
type TUIApprover struct {
	mu sync.RWMutex
	p  *tea.Program
}

func NewTUIApprover() *TUIApprover {
	return &TUIApprover{}
}

func (a *TUIApprover) SetProgram(p *tea.Program) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.p = p
}

// Send dispatches a message to the active Bubble Tea program if it is running.
func (a *TUIApprover) Send(msg tea.Msg) {
	a.mu.RLock()
	p := a.p
	a.mu.RUnlock()
	if p != nil {
		p.Send(msg)
	}
}

// RequestApproval posts an approval request to the Bubble Tea event loop and blocks until
// the user responds (y/n) or the context is cancelled.
func (a *TUIApprover) RequestApproval(ctx context.Context, req permissions.Request) (bool, error) {
	a.mu.RLock()
	p := a.p
	a.mu.RUnlock()

	if p == nil {
		return false, fmt.Errorf("TUI program is not active")
	}

	respCh := make(chan bool, 1)

	p.Send(approvalPromptMsg{
		req:    req,
		respCh: respCh,
	})

	select {
	case approved := <-respCh:
		return approved, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}
