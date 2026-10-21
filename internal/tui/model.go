package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/Aneeshie/zerrcode/internal/agent"
	"github.com/Aneeshie/zerrcode/internal/client"
	"github.com/Aneeshie/zerrcode/internal/permissions"
	"github.com/charmbracelet/x/ansi"
)

type itemType int

const (
	itemUser itemType = iota
	itemAssistant
	itemTool
	itemError
	itemSystem
)

type chatItem struct {
	kind     itemType
	title    string
	content  string
	toolName string
}

// Background messages for Bubble Tea event loop
type agentChunkMsg string
type agentEventMsg agent.Event
type agentDoneMsg struct {
	updatedHistory []client.Message
	err            error
}

type pendingApproval struct {
	req        permissions.Request
	respCh     chan bool
	showDiff   bool
	activeFile int
}

// Model represents the minimal full-screen TUI state.
type Model struct {
	agent           *agent.Agent
	history         []client.Message
	input           string
	items           []chatItem
	loading         bool
	err             error
	quitting        bool
	width           int
	height          int
	scrollOffset    int
	autoScroll      bool
	workspace       string
	pendingApproval *pendingApproval
	cancelTurn      context.CancelFunc
	approver        *TUIApprover
	streamingActive bool
}

// NewModel initializes the minimal TUI.
func NewModel(ag *agent.Agent, initialHistory []client.Message, approver *TUIApprover) Model {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	base := filepath.Base(cwd)

	if approver == nil {
		approver = NewTUIApprover()
	}

	return Model{
		agent:      ag,
		history:    initialHistory,
		autoScroll: true,
		workspace:  base,
		width:      100,
		height:     30,
		approver:   approver,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case approvalPromptMsg:
		m.pendingApproval = &pendingApproval{
			req:      msg.req,
			respCh:   msg.respCh,
			showDiff: true,
		}
		m.autoScroll = true
		return m, nil

	case agentChunkMsg:
		// Stream assistant token chunks
		chunk := string(msg)
		if len(m.items) > 0 && m.streamingActive && m.items[len(m.items)-1].kind == itemAssistant {
			m.items[len(m.items)-1].content += chunk
		} else {
			m.items = append(m.items, chatItem{
				kind:    itemAssistant,
				title:   "zerrcode",
				content: chunk,
			})
			m.streamingActive = true
		}
		m.autoScroll = true
		return m, nil

	case agentEventMsg:
		e := agent.Event(msg)
		switch e.Kind {
		case agent.EventToolStart:
			m.streamingActive = false
			line := fmt.Sprintf("⚡ %s", e.Tool)
			if strings.TrimSpace(e.Args) != "" {
				line += " " + e.Args
			}
			m.items = append(m.items, chatItem{
				kind:     itemTool,
				toolName: e.Tool,
				content:  line,
			})

		case agent.EventToolResult:
			m.streamingActive = false
			statusPrefix := "  ↳"
			switch e.Status {
			case agent.StatusRejected:
				statusPrefix = "  ✕ (rejected by user)"
			case agent.StatusFailed:
				statusPrefix = fmt.Sprintf("  ⚠ failed: %v", e.Err)
			case agent.StatusSucceeded:
				out := strings.TrimSpace(e.Output)
				firstLine := out
				if idx := strings.IndexByte(out, '\n'); idx >= 0 {
					firstLine = out[:idx]
				}
				if len(firstLine) > 70 {
					firstLine = firstLine[:70] + "..."
				}
				if firstLine != "" {
					statusPrefix = fmt.Sprintf("  ↳ %s", firstLine)
				} else {
					statusPrefix = "  ✓ completed"
				}
			}
			m.items = append(m.items, chatItem{
				kind:    itemTool,
				content: statusPrefix,
			})

		case agent.EventVerify:
			m.streamingActive = false
			m.items = append(m.items, chatItem{
				kind:    itemTool,
				content: "  🔍 inspecting workspace git diff to verify changes...",
			})

		case agent.EventAssistant:
			// If not already streamed or need final update
			if !m.streamingActive && e.Output != "" {
				m.items = append(m.items, chatItem{
					kind:    itemAssistant,
					title:   "zerrcode",
					content: e.Output,
				})
			}
			m.streamingActive = false
		}
		m.autoScroll = true
		return m, nil

	case agentDoneMsg:
		m.loading = false
		m.streamingActive = false
		m.cancelTurn = nil
		m.autoScroll = true

		if msg.err != nil {
			if msg.err == context.Canceled {
				m.items = append(m.items, chatItem{
					kind:    itemSystem,
					title:   "CANCELLED",
					content: "Turn cancelled by user.",
				})
			} else {
				m.err = msg.err
				m.items = append(m.items, chatItem{
					kind:    itemError,
					title:   "Error",
					content: fmt.Sprintf("%v", msg.err),
				})
			}
			return m, nil
		}

		m.history = msg.updatedHistory
		return m, nil

	case tea.KeyPressMsg:
		// 1. Approval mode key handling
		if m.pendingApproval != nil {
			switch msg.String() {
			case "y", "Y":
				m.pendingApproval.respCh <- true
				m.items = append(m.items, chatItem{
					kind:    itemTool,
					content: fmt.Sprintf("✓ Approved %s", m.pendingApproval.req.Tool),
				})
				m.pendingApproval = nil
				m.autoScroll = true
				return m, nil

			case "n", "N":
				m.pendingApproval.respCh <- false
				m.items = append(m.items, chatItem{
					kind:    itemTool,
					content: fmt.Sprintf("✕ Rejected %s", m.pendingApproval.req.Tool),
				})
				m.pendingApproval = nil
				m.autoScroll = true
				return m, nil

			case "d", "D":
				m.pendingApproval.showDiff = !m.pendingApproval.showDiff
				return m, nil

			case "tab":
				if len(m.pendingApproval.req.Changes) > 1 {
					m.pendingApproval.activeFile = (m.pendingApproval.activeFile + 1) % len(m.pendingApproval.req.Changes)
				}
				return m, nil

			case "ctrl+c", "esc":
				m.pendingApproval.respCh <- false
				m.pendingApproval = nil
				if m.cancelTurn != nil {
					m.cancelTurn()
				}
				m.loading = false
				m.items = append(m.items, chatItem{
					kind:    itemSystem,
					title:   "CANCELLED",
					content: "Operation and turn cancelled.",
				})
				m.autoScroll = true
				return m, nil

			case "up", "k":
				m.autoScroll = false
				if m.scrollOffset > 0 {
					m.scrollOffset--
				}
				return m, nil

			case "down", "j":
				m.scrollOffset++
				return m, nil

			default:
				return m, nil
			}
		}

		// 2. Normal interaction mode
		switch msg.String() {
		case "ctrl+c":
			if m.loading {
				if m.cancelTurn != nil {
					m.cancelTurn()
				}
				m.loading = false
				m.items = append(m.items, chatItem{
					kind:    itemSystem,
					title:   "CANCELLED",
					content: "Turn cancelled by user.",
				})
				m.autoScroll = true
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case "esc":
			if m.input != "" {
				m.input = ""
				return m, nil
			}
			if m.loading {
				if m.cancelTurn != nil {
					m.cancelTurn()
				}
				m.loading = false
				m.items = append(m.items, chatItem{
					kind:    itemSystem,
					title:   "CANCELLED",
					content: "Turn cancelled by user.",
				})
				m.autoScroll = true
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			m.autoScroll = false
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
			return m, nil

		case "down", "j":
			m.scrollOffset++
			return m, nil

		case "pgup":
			m.autoScroll = false
			m.scrollOffset -= 5
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
			return m, nil

		case "pgdown":
			m.scrollOffset += 5
			return m, nil

		case "enter":
			if strings.TrimSpace(m.input) == "" || m.loading {
				return m, nil
			}

			prompt := strings.TrimSpace(m.input)
			m.input = ""

			switch prompt {
			case "/clear":
				m.items = nil
				m.scrollOffset = 0
				m.autoScroll = true
				return m, nil
			case "/exit", "/quit":
				m.quitting = true
				return m, tea.Quit
			case "/help":
				m.items = append(m.items, chatItem{
					kind:    itemSystem,
					title:   "Help",
					content: "Commands:\n  /clear  - Clear conversation view\n  /exit   - Exit application\n  ↑ / ↓   - Scroll history\n  Esc     - Clear input or cancel\n  Ctrl+C  - Cancel turn or quit",
				})
				m.autoScroll = true
				return m, nil
			}

			m.items = append(m.items, chatItem{
				kind:    itemUser,
				title:   "You",
				content: prompt,
			})

			m.loading = true
			m.err = nil
			m.autoScroll = true
			m.streamingActive = false

			m.history = append(m.history, client.Message{
				Role:    client.RoleUser,
				Content: prompt,
			})

			turnCtx, turnCancel := context.WithCancel(context.Background())
			m.cancelTurn = turnCancel

			return m, runAgentTurn(m.agent, m.history, turnCtx, m.approver)

		case "backspace":
			runes := []rune(m.input)
			if len(runes) > 0 {
				m.input = string(runes[:len(runes)-1])
			}
			return m, nil

		default:
			if text := msg.Key().Text; text != "" {
				m.input += text
			}
			return m, nil
		}
	}

	return m, nil
}

func (m Model) View() tea.View {
	if m.quitting {
		v := tea.NewView(fg(colMuted, "Goodbye.\n"))
		v.AltScreen = false
		return v
	}

	width := m.width
	if width <= 0 {
		width = 80
	}
	height := m.height
	if height <= 0 {
		height = 24
	}

	var sb strings.Builder

	// 1. Header (2 lines)
	sb.WriteString(m.renderHeader(width) + "\n")
	sb.WriteString(divider(width) + "\n")

	// Dimensions
	headerHeight := 2
	composerHeight := 4
	footerHeight := 1
	viewportHeight := height - headerHeight - composerHeight - footerHeight
	if viewportHeight < 3 {
		viewportHeight = 3
	}

	// 2. Chat Viewport Lines
	chatLines := m.renderChatLines(width)

	maxScroll := len(chatLines) - viewportHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	if m.autoScroll || m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	end := m.scrollOffset + viewportHeight
	if end > len(chatLines) {
		end = len(chatLines)
	}

	visibleLines := chatLines[m.scrollOffset:end]
	for _, l := range visibleLines {
		sb.WriteString(l + "\n")
	}

	for i := len(visibleLines); i < viewportHeight; i++ {
		sb.WriteString("\n")
	}

	// 3. Input Composer (3 lines)
	composerLines := m.renderComposer(width)
	for _, l := range composerLines {
		sb.WriteString(l + "\n")
	}

	// 4. Footer (1 line)
	sb.WriteString(m.renderFooter(width))

	view := tea.NewView(sb.String())
	view.AltScreen = true
	return view
}

func (m Model) renderHeader(width int) string {
	brand := bold + fg(colBrand, "zerrcode")
	sub := dim + fg(colMuted, " · "+m.workspace)

	var status string
	if m.pendingApproval != nil {
		status = bold + fg(colTool, "⚠️  APPROVAL REQUIRED")
	} else if m.loading {
		status = fg(colTool, "◐ thinking...")
	} else {
		status = fg(colGreen, "● ready")
	}

	left := brand + sub
	spaceLen := width - ansi.StringWidth(left) - ansi.StringWidth(status) - 2
	if spaceLen < 1 {
		spaceLen = 1
	}

	return " " + left + strings.Repeat(" ", spaceLen) + status + " "
}

func (m Model) renderChatLines(width int) []string {
	var lines []string

	if len(m.items) == 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+dim+fg(colMuted, "Ask a question about your code, or type /help"))
		return lines
	}

	contentWidth := width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}

	for _, item := range m.items {
		switch item.kind {
		case itemUser:
			lines = append(lines, "")
			lines = append(lines, " "+bold+fg(colUser, item.title))
			lines = append(lines, wrapLines(item.content, contentWidth, "   ")...)

		case itemAssistant:
			lines = append(lines, "")
			lines = append(lines, " "+bold+fg(colBrand, item.title))
			lines = append(lines, wrapLines(item.content, contentWidth, "   ")...)

		case itemTool:
			lines = append(lines, dim+fg(colMuted, "  "+item.content))

		case itemError:
			lines = append(lines, "")
			lines = append(lines, " "+bold+fg(colRed, "Error"))
			lines = append(lines, wrapLines(item.content, contentWidth, "   ")...)

		default:
			lines = append(lines, "")
			lines = append(lines, " "+dim+fg(colMuted, item.title))
			lines = append(lines, wrapLines(item.content, contentWidth, "   ")...)
		}
	}

	// Live approval inspection card
	if m.pendingApproval != nil {
		lines = append(lines, "")
		req := m.pendingApproval.req
		lines = append(lines, " "+bold+fg(colTool, fmt.Sprintf("⚠️  Proposed %s (%d file(s) changed):", req.Tool, len(req.Changes))))

		for i, c := range req.Changes {
			activeIndicator := "  • "
			if i == m.pendingApproval.activeFile {
				activeIndicator = "  ➜ "
			}
			summary := formatChangeSummary(c)
			if i == m.pendingApproval.activeFile {
				lines = append(lines, bold+fg(colText, activeIndicator+summary))
			} else {
				lines = append(lines, dim+fg(colMuted, activeIndicator+summary))
			}
		}

		if m.pendingApproval.showDiff && len(req.Changes) > 0 {
			curIdx := m.pendingApproval.activeFile
			if curIdx >= len(req.Changes) {
				curIdx = 0
			}
			curChange := req.Changes[curIdx]
			lines = append(lines, "")
			lines = append(lines, dim+fg(colMuted, fmt.Sprintf("  --- Diff preview: %s (File %d of %d, [Tab] to switch) ---", curChange.Path, curIdx+1, len(req.Changes))))
			lines = append(lines, renderDiffLines(curChange.Diff, contentWidth, "  ")...)
		}
	} else if m.loading {
		lines = append(lines, "")
		lines = append(lines, "  "+dim+fg(colTool, "⏳ running tools & thinking..."))
	}

	return lines
}

func (m Model) renderComposer(width int) []string {
	boxWidth := width - 2
	if boxWidth < 20 {
		boxWidth = 20
	}
	innerWidth := boxWidth - 2

	borderCol := colBorder
	var displayText string

	if m.pendingApproval != nil {
		borderCol = colTool
		displayText = bold + fg(colTool, "Approve changes? [y] Yes  ·  [n] No  ·  [d] Toggle Diff  ·  [Tab] Next File")
	} else if m.loading {
		borderCol = colTool
		displayText = ital + dim + fg(colMuted, "zerrcode is working... (Ctrl+C / Esc to cancel)")
	} else {
		if m.input != "" {
			borderCol = colBrand
		}
		cursor := bold + fg(colBrand, "▌")
		if m.input == "" {
			displayText = dim + fg(colMuted, "Send a message...") + cursor
		} else {
			displayText = m.input + cursor
		}
	}

	top := borderCol + "╭" + strings.Repeat("─", innerWidth+2) + "╮" + reset

	padLen := innerWidth - ansi.StringWidth(displayText) - 3
	if padLen < 0 {
		padLen = 0
	}

	promptPrefix := fg(colUser, "❯ ")
	if m.pendingApproval != nil {
		promptPrefix = fg(colTool, "⚠ ")
	}

	mid := borderCol + "│ " + reset + promptPrefix + displayText + strings.Repeat(" ", padLen) + borderCol + " │" + reset
	bottom := borderCol + "╰" + strings.Repeat("─", innerWidth+2) + "╯" + reset

	return []string{top, mid, bottom}
}

func (m Model) renderFooter(width int) string {
	var shortcuts string
	if m.pendingApproval != nil {
		shortcuts = dim + fg(colMuted, " [y] approve  ·  [n] reject  ·  [d] toggle diff  ·  [Tab] next file")
	} else if m.loading {
		shortcuts = dim + fg(colMuted, " [Ctrl+C] Cancel turn  ·  [↑/↓] Scroll")
	} else {
		shortcuts = dim + fg(colMuted, " esc quit  ·  enter send  ·  ↑/↓ scroll  ·  /help  ·  /clear")
	}
	return " " + shortcuts
}

// runAgentTurn runs the agent in a background goroutine and connects real-time streaming & events.
func runAgentTurn(ag *agent.Agent, history []client.Message, ctx context.Context, approver *TUIApprover) tea.Cmd {
	return func() tea.Msg {
		if ag == nil {
			return agentDoneMsg{
				updatedHistory: history,
				err:            fmt.Errorf("agent is not configured"),
			}
		}

		onText := func(chunk string) {
			if approver != nil {
				approver.Send(agentChunkMsg(chunk))
			}
		}

		updatedHistory, runErr := ag.Run(ctx, history, onText)

		return agentDoneMsg{
			updatedHistory: updatedHistory,
			err:            runErr,
		}
	}
}

// Run launches the Bubble Tea program with the interactive approver attached.
func Run(ag *agent.Agent, history []client.Message) error {
	approver := NewTUIApprover()
	if ag != nil {
		ag.SetApprover(approver)
	}

	m := NewModel(ag, history, approver)
	p := tea.NewProgram(m)
	approver.SetProgram(p)

	if ag != nil {
		ag.OnEvent = func(e agent.Event) {
			approver.Send(agentEventMsg(e))
		}
	}

	_, err := p.Run()
	return err
}
