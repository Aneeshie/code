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

type Agent struct {
	client   client.LLMClient
	registry *tools.Registry

	// MaxIterations caps model round-trips per Run. Values <= 0 mean no cap.
	MaxIterations int
}

func NewAgent(client client.LLMClient, registry *tools.Registry) *Agent {
	return &Agent{
		client:        client,
		registry:      registry,
		MaxIterations: DefaultMaxIterations,
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

		// No tool call means the agent has finished
		if len(result.Message.ToolCalls) == 0 {
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
			}

			history = append(history, client.Message{Content: output, Role: client.RoleTool, ToolCallID: call.ID})

		}

	}

	return nil, fmt.Errorf("%w (%d)", ErrMaxIterations, a.MaxIterations)
}
