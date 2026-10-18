package client

import "context"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

type Message struct {
	Content    string
	Role       Role
	ToolCalls  []ToolCall
	ToolCallID string
}

type Response struct {
	Content   string
	ToolCalls []ToolCall
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type StreamEvent struct {
	ID      string
	Message Message
	Err     error
}

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type LLMClient interface {
	Send(ctx context.Context, messages []Message) (*Response, error)
	Stream(ctx context.Context, messages []Message, definitions []ToolDefinition, onText func(string)) (*StreamEvent, error)
}
