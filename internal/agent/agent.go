package agent

import (
	"context"
	"fmt"

	"github.com/Aneeshie/zerrcode/internal/client"
	"github.com/Aneeshie/zerrcode/internal/tools"
)

type Agent struct {
	client   client.LLMClient
	registry *tools.Registry
}

func NewAgent(client client.LLMClient, registry *tools.Registry) *Agent {
	return &Agent{
		client:   client,
		registry: registry,
	}
}

func (a *Agent) Run(ctx context.Context, history []client.Message, onText func(string)) ([]client.Message, error) {
	for {
		result, err := a.client.Stream(ctx, history, onText)
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
}
