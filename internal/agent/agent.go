package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/Aneeshie/zerrcode/internal/client"
	"github.com/Aneeshie/zerrcode/internal/tools"
)

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

		// No tool call means the agent wants to finish
		if len(result.Message.ToolCalls) == 0 {
			if unverified && nudges < a.MaxVerificationNudges {
				nudges++
				history = append(history, client.Message{Role: client.RoleUser, Content: verificationReminder})
				continue
			}
			return history, nil
		}

		// Execute tool calls and update history
		for _, call := range result.Message.ToolCalls {
			output, err := a.registry.Execute(
				ctx,
				call.Name,
				call.Arguments,
			)

			fmt.Printf(
				"\n[Tool: %s | Args: %q]\n",
				call.Name,
				call.Arguments,
			)

			if err != nil {
				fmt.Printf("[Tool error: %v]\n", err)
				output = fmt.Sprintf("Tool call failed: %v", err)
			} else if a.registry.IsMutating(call.Name) {
				unverified = true
			} else if call.Name == VerificationTool {
				unverified = false
			}

			history = append(history, client.Message{Content: output, Role: client.RoleTool, ToolCallID: call.ID})

		}

	}

	return nil, fmt.Errorf("%w (%d)", ErrMaxIterations, a.MaxIterations)
}
