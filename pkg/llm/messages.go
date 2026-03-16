package llm

import (
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func UserMessage(content string) Message {
	return Message{Role: RoleUser, Content: content}
}

func SystemMessage(content string) Message {
	return Message{Role: RoleSystem, Content: content}
}

func AssistantMessage(content string) Message {
	return Message{Role: RoleAssistant, Content: content}
}

func AssistantMessageWithTools(content, reasoningContent string, toolCalls []tools.ToolCall) Message {
	return Message{
		Role:             RoleAssistant,
		Content:          content,
		ReasoningContent: reasoningContent,
		ToolCalls:        toolCalls,
	}
}

func ToolResultMessage(toolCallID, name, content string) Message {
	return Message{
		Role:       RoleTool,
		ToolCallID: toolCallID,
		Name:       name,
		Content:    content,
	}
}
