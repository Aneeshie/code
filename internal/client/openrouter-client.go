package client

import (
	"context"
	"fmt"
	"strings"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
)

type OpenRouterClient struct {
	Client *openrouter.OpenRouter
}

func NewOpenRouterClient(apiKey string) *OpenRouterClient {
	return &OpenRouterClient{
		Client: openrouter.New(
			openrouter.WithSecurity(apiKey),
		),
	}
}

func (o *OpenRouterClient) Send(ctx context.Context, messages []Message) (*Response, error) {
	chatReq := convertToChatRequest(messages, nil)

	res, err := o.Client.Chat.Send(ctx, chatReq, nil)
	if err != nil {
		return nil, err
	}

	response := convertResponse(res.ChatResult.Choices[0].Message)

	return &response, nil

}

func (o *OpenRouterClient) Stream(ctx context.Context, messages []Message, definitions []ToolDefinition, onText func(string)) (*StreamEvent, error) {
	chatReq := convertToChatRequest(messages, definitions)
	res, err := o.Client.Chat.Send(ctx, chatReq, nil)
	if err != nil {
		return nil, err
	}

	if res == nil {
		return nil, fmt.Errorf("OpenRouter returned an empty response")
	}
	if res.EventStream == nil {
		return nil, fmt.Errorf("OpenRouter returned no event stream")
	}

	defer res.EventStream.Close()

	var text strings.Builder

	var responseID string

	toolCalls := make(map[int64]*ToolCall)

	for res.EventStream.Next() {
		event := res.EventStream.Value()
		if event == nil || len(event.Data.Choices) == 0 {
			continue
		}

		if responseID == "" {
			responseID = event.Data.ID
		}

		delta := event.Data.Choices[0].Delta

		for _, tc := range delta.ToolCalls {
			call, ok := toolCalls[tc.Index]

			if !ok {
				call = &ToolCall{}
				toolCalls[tc.Index] = call
			}

			if tc.ID != nil {
				call.ID = *tc.ID
			}

			if tc.Function != nil {
				if tc.Function.Name != nil {
					call.Name = *tc.Function.Name
				}
				if tc.Function.Arguments != nil {
					call.Arguments += *tc.Function.Arguments
				}
			}
		}

		content, ok := delta.Content.Get()

		if ok && content != nil {
			onText(*content)
			text.WriteString(*content)
		}
	}

	if err := res.EventStream.Err(); err != nil {
		return nil, err
	}

	return &StreamEvent{
		ID: responseID,
		Message: Message{
			Role:      RoleAssistant,
			Content:   text.String(),
			ToolCalls: collectToolCalls(toolCalls),
		},
	}, nil

}

func collectToolCalls(calls map[int64]*ToolCall) []ToolCall {

	result := make([]ToolCall, 0, len(calls))

	for i := int64(0); i < int64(len(calls)); i++ {
		if call, ok := calls[i]; ok {
			result = append(result, *call)
		}
	}

	return result
}

func convertResponse(message components.ChatAssistantMessage) Response {
	var sdkToolCalls []ToolCall
	// extract generated string.
	generatedString, ok := message.Content.Get()

	// convert the SDK tool calls.
	for _, toolCall := range message.ToolCalls {
		sdkToolCall := ToolCall{
			ID:        toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		}

		sdkToolCalls = append(sdkToolCalls, sdkToolCall)
	}

	if !ok || generatedString.Str == nil {
		return Response{
			Content:   "",
			ToolCalls: sdkToolCalls,
		}
	}

	//return internal response
	return Response{
		Content:   *generatedString.Str,
		ToolCalls: sdkToolCalls,
	}
}

func convertToChatRequest(messages []Message, definitions []ToolDefinition) components.ChatRequest {
	var msgs []components.ChatMessages
	model := "nvidia/nemotron-3.5-super-vl:free"
	stream := true

	for _, message := range messages {
		var msg components.ChatMessages

		switch message.Role {
		case RoleUser:
			msg = components.CreateChatMessagesUser(
				components.ChatUserMessage{
					Content: components.CreateChatUserMessageContentStr(message.Content),
					Role:    components.ChatUserMessageRoleUser,
				},
			)
		case RoleAssistant:
			var sdkToolCalls []components.ChatToolCall

			for _, toolCall := range message.ToolCalls {
				sdkToolCalls = append(sdkToolCalls,
					components.ChatToolCall{
						ID:   toolCall.ID,
						Type: components.ChatToolCallTypeFunction,
						Function: components.ChatToolCallFunction{
							Name:      toolCall.Name,
							Arguments: toolCall.Arguments,
						},
					},
				)
			}

			msg = components.CreateChatMessagesAssistant(
				components.ChatAssistantMessage{
					Content: optionalnullable.From(
						func() *components.ChatAssistantMessageContent {
							content := components.CreateChatAssistantMessageContentStr(message.Content)
							return &content
						}(),
					),
					Role:      components.ChatAssistantMessageRole(message.Role),
					ToolCalls: sdkToolCalls,
				},
			)
		case RoleSystem:
			msg = components.CreateChatMessagesSystem(
				components.ChatSystemMessage{
					Content: components.CreateChatSystemMessageContentStr(message.Content),
					Role:    components.ChatSystemMessageRoleSystem,
				},
			)

		case RoleTool:
			content := message.Content

			msg = components.CreateChatMessagesTool(
				components.ChatToolMessage{
					Content:    components.CreateChatToolMessageContentStr(content),
					Role:       components.ChatToolMessageRoleTool,
					ToolCallID: message.ToolCallID,
				},
			)

		default:
			continue
		}

		msgs = append(msgs, msg)

	}

	return components.ChatRequest{
		Messages: msgs,
		Model:    &model,
		Stream:   &stream,
		Tools:    convertToolDefinitions(definitions),
	}
}

func convertToolDefinitions(definitions []ToolDefinition) []components.ChatFunctionTool {
	tools := make([]components.ChatFunctionTool, 0, len(definitions))

	for _, def := range definitions {
		description := def.Description
		tools = append(tools, components.ChatFunctionTool{
			ChatFunctionToolFunction: &components.ChatFunctionToolFunction{
				Type: components.ChatFunctionToolTypeFunction,
				Function: components.ChatFunctionToolFunctionFunction{
					Name:        def.Name,
					Description: &description,
					Parameters:  def.InputSchema,
				},
			},
			Type: components.ChatFunctionToolUnionTypeChatFunctionToolFunction,
		})
	}

	return tools
}
