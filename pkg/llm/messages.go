package llm

import (
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// IsMultimodal reports whether this message carries content parts
// rather than plain text.
func (m Message) IsMultimodal() bool {
	return len(m.Parts) > 0
}

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

// UserMessageWithParts builds a multimodal user message.
// Use when the user's turn includes files alongside text.
func UserMessageWithParts(parts []ContentPart) Message {
	return Message{
		Role:  RoleUser,
		Parts: parts,
	}
}
