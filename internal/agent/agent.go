package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/Aneeshie/zerrcode/internal/client"
	"github.com/Aneeshie/zerrcode/internal/permissions"
	"github.com/Aneeshie/zerrcode/internal/tools"
)

// EventKind identifies what an Event reports.
type EventKind int

const (
	// EventAssistant is emitted after each complete model response. Output is
	// the response text (possibly empty when the model only called tools).
	EventAssistant EventKind = iota
	// EventToolStart is emitted when a tool call begins, before any approval.
	EventToolStart
	// EventToolResult is emitted when a tool call finishes.
	EventToolResult
	// EventVerify is emitted when the agent sends the model back to run
	// git_diff before it may finish.
	EventVerify
)

// ToolStatus is the outcome of a finished tool call.
type ToolStatus int

const (
	StatusSucceeded ToolStatus = iota
	StatusFailed
	// StatusRejected means the user declined the change; nothing was written.
	StatusRejected
)

// Event lets a UI observe a run without the agent printing anywhere itself.
type Event struct {
	Kind   EventKind
	CallID string
	Tool   string
	Args   string
	Output string
	Err    error
	Status ToolStatus
}

// DefaultMaxIterations bounds how many model round-trips a single Run may make.
const DefaultMaxIterations = 25

// ErrMaxIterations is returned when the model keeps requesting tool calls
// past the iteration limit.
var ErrMaxIterations = errors.New("agent exceeded maximum tool-call iterations")

// VerificationTool is the tool the agent must call, after changing files and
// before reporting completion, to inspect what actually changed.
const VerificationTool = "git_diff"

// DefaultMaxVerificationNudges bounds how many times a Run will send the model
// back to verify its changes before giving up and returning anyway.
const DefaultMaxVerificationNudges = 2

const verificationReminder = "You changed files but have not inspected the result yet. Before reporting completion, call " + VerificationTool +
	" and check that the list of changed files is exactly what you intended and that the diff matches your intent. " +
	"Fix anything unexpected. In your final report, say that you inspected the diff, summarise the changed files, " +
	"and list separately anything you did not run or verify."

type Agent struct {
	client   client.LLMClient
	registry *tools.Registry

	// MaxIterations caps model round-trips per Run. Values <= 0 mean no cap.
	MaxIterations int

	// MaxVerificationNudges caps how many times the agent is sent back to run
	// VerificationTool when it tries to finish with unverified changes.
	// Values <= 0 disable the check.
	MaxVerificationNudges int

	// OnEvent, if set, is called synchronously from Run for each Event. It must
	// not block for long.
	OnEvent func(Event)
}

// SetApprover configures who approves workspace-changing tool calls. The
// registry refuses such calls when no approver is set.
func (a *Agent) SetApprover(m permissions.Manager) { a.registry.SetApprover(m) }

func (a *Agent) emit(e Event) {
	if a.OnEvent != nil {
		a.OnEvent(e)
	}
}

func NewAgent(client client.LLMClient, registry *tools.Registry) *Agent {
	return &Agent{
		client:        client,
		registry:      registry,
		MaxIterations: DefaultMaxIterations,

		MaxVerificationNudges: DefaultMaxVerificationNudges,
	}
}

func (a *Agent) Run(ctx context.Context, history []client.Message, onText func(string)) ([]client.Message, error) {
	var definitions []client.ToolDefinition
	for _, tool := range a.registry.All() {
		definitions = append(definitions, client.ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		})
	}

	// unverified is true while there are successful workspace changes that
	// VerificationTool has not been called on since.
	unverified := false
	nudges := 0

	for iteration := 0; a.MaxIterations <= 0 || iteration < a.MaxIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		result, err := a.client.Stream(ctx, history, definitions, onText)
		if err != nil {
			return nil, err
		}
		// preserve the assistant's response and any tool calls
		history = append(history, result.Message)
		a.emit(Event{Kind: EventAssistant, Output: result.Message.Content})

		// No tool call means the agent wants to finish
		if len(result.Message.ToolCalls) == 0 {
			if unverified && nudges < a.MaxVerificationNudges {
				nudges++
				history = append(history, client.Message{Role: client.RoleUser, Content: verificationReminder})
				a.emit(Event{Kind: EventVerify})
				continue
			}
			return history, nil
		}

		// Execute tool calls and update history
		for _, call := range result.Message.ToolCalls {
			a.emit(Event{Kind: EventToolStart, CallID: call.ID, Tool: call.Name, Args: call.Arguments})

			output, err := a.registry.Execute(
				tools.WithCallID(ctx, call.ID),
				call.Name,
				call.Arguments,
			)

			status := StatusSucceeded
			if err != nil {
				status = StatusFailed
				if errors.Is(err, tools.ErrRejected) {
					status = StatusRejected
				}
				output = fmt.Sprintf("Tool call failed: %v", err)
			} else if a.registry.IsMutating(call.Name) {
				unverified = true
			} else if call.Name == VerificationTool {
				unverified = false
			}

			a.emit(Event{Kind: EventToolResult, CallID: call.ID, Tool: call.Name, Args: call.Arguments, Output: output, Err: err, Status: status})

			history = append(history, client.Message{Content: output, Role: client.RoleTool, ToolCallID: call.ID})

		}

	}

	return nil, fmt.Errorf("%w (%d)", ErrMaxIterations, a.MaxIterations)
}
