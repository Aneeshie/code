package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Aneeshie/zerrcode/internal/client"
	"github.com/Aneeshie/zerrcode/internal/diff"
	"github.com/Aneeshie/zerrcode/internal/permissions"
)

func TestModelTypingAndBackspace(t *testing.T) {
	m := NewModel(nil, nil, nil)

	// Simulate typing 'h', 'i'
	updated, _ := m.Update(tea.KeyPressMsg{Text: "h"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "i"})
	m = updated.(Model)

	if m.input != "hi" {
		t.Fatalf("expected input 'hi', got %q", m.input)
	}

	// Simulate backspace
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = updated.(Model)

	if m.input != "h" {
		t.Fatalf("expected input 'h' after backspace, got %q", m.input)
	}
}

func TestModelQuit(t *testing.T) {
	m := NewModel(nil, nil, nil)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)

	if !m.quitting {
		t.Fatalf("expected quitting to be true")
	}
	if cmd == nil {
		t.Fatalf("expected quit command")
	}

	view := m.View()
	if !strings.Contains(view.Content, "Goodbye") {
		t.Fatalf("expected Goodbye in view, got %q", view.Content)
	}
}

func TestModelAgentDone(t *testing.T) {
	m := NewModel(nil, nil, nil)
	m.loading = true

	done := agentDoneMsg{
		updatedHistory: []client.Message{
			{Role: client.RoleUser, Content: "Hello"},
			{Role: client.RoleAssistant, Content: "Hi there!"},
		},
	}

	updated, _ := m.Update(done)
	m = updated.(Model)

	if m.loading {
		t.Fatalf("expected loading to be false")
	}
	if len(m.history) != 2 {
		t.Fatalf("expected 2 messages in history, got %d", len(m.history))
	}
}

func TestModelStreaming(t *testing.T) {
	m := NewModel(nil, nil, nil)

	// Chunk 1
	updated, _ := m.Update(agentChunkMsg("Hello"))
	m = updated.(Model)

	// Chunk 2
	updated, _ = m.Update(agentChunkMsg(" world!"))
	m = updated.(Model)

	if len(m.items) != 1 || m.items[0].kind != itemAssistant {
		t.Fatalf("expected 1 assistant item, got %+v", m.items)
	}
	if m.items[0].content != "Hello world!" {
		t.Fatalf("expected 'Hello world!', got %q", m.items[0].content)
	}
}

func TestModelApprovalFlow(t *testing.T) {
	m := NewModel(nil, nil, nil)
	respCh := make(chan bool, 1)

	req := permissions.Request{
		Tool: "write_file",
		Changes: []permissions.FileChange{
			{
				Path: "main.go",
				Op:   permissions.OpModify,
				Diff: diff.Compute("var a = 1\n", "var a = 2\n"),
			},
			{
				Path: "hello.go",
				Op:   permissions.OpCreate,
				Diff: diff.Compute("", "package main\n"),
			},
		},
	}

	// 1. Receive approval prompt
	updated, _ := m.Update(approvalPromptMsg{req: req, respCh: respCh})
	m = updated.(Model)

	if m.pendingApproval == nil {
		t.Fatalf("expected pendingApproval to be set")
	}

	// 2. Test toggling diff
	updated, _ = m.Update(tea.KeyPressMsg{Text: "d"})
	m = updated.(Model)
	if m.pendingApproval.showDiff {
		t.Fatalf("expected showDiff to be false after toggle")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Text: "d"})
	m = updated.(Model)
	if !m.pendingApproval.showDiff {
		t.Fatalf("expected showDiff to be true after toggle")
	}

	// 3. Test cycling files with tab
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if m.pendingApproval.activeFile != 1 {
		t.Fatalf("expected activeFile 1, got %d", m.pendingApproval.activeFile)
	}

	// 4. Test approve with 'y'
	updated, _ = m.Update(tea.KeyPressMsg{Text: "y"})
	m = updated.(Model)

	if m.pendingApproval != nil {
		t.Fatalf("expected pendingApproval to be cleared after approve")
	}

	select {
	case approved := <-respCh:
		if !approved {
			t.Fatalf("expected approved to be true")
		}
	default:
		t.Fatalf("expected response on respCh")
	}
}

func TestModelApprovalReject(t *testing.T) {
	m := NewModel(nil, nil, nil)
	respCh := make(chan bool, 1)

	req := permissions.Request{Tool: "delete_file"}
	updated, _ := m.Update(approvalPromptMsg{req: req, respCh: respCh})
	m = updated.(Model)

	// Reject with 'n'
	updated, _ = m.Update(tea.KeyPressMsg{Text: "n"})
	m = updated.(Model)

	if m.pendingApproval != nil {
		t.Fatalf("expected pendingApproval to be cleared after reject")
	}

	select {
	case approved := <-respCh:
		if approved {
			t.Fatalf("expected approved to be false")
		}
	default:
		t.Fatalf("expected response on respCh")
	}
}

func TestModelSlashCommands(t *testing.T) {
	m := NewModel(nil, nil, nil)

	// Test /help
	m.input = "/help"
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if len(m.items) != 1 || m.items[0].kind != itemSystem {
		t.Fatalf("expected help item, got %+v", m.items)
	}

	// Test /clear
	m.input = "/clear"
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if len(m.items) != 0 {
		t.Fatalf("expected items to be cleared, got %+v", m.items)
	}
}

func TestModelWindowResize(t *testing.T) {
	m := NewModel(nil, nil, nil)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if m.width != 120 || m.height != 40 {
		t.Fatalf("expected 120x40, got %dx%d", m.width, m.height)
	}

	view := m.View()
	if !view.AltScreen {
		t.Fatalf("expected AltScreen to be true for fullscreen rendering")
	}
}
