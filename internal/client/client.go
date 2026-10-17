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
	Content   string
	Role      Role
	ToolCalls []ToolCall
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

type LLMClient interface {
	Send(ctx context.Context, messages []Message) (*Response, error)
	Stream(ctx context.Context, messages []Message, onText func(string)) error
}
