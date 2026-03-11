// Package llm provides LLM client abstractions for OnlyAgents
package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGemini    Provider = "gemini"
)

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// HasToolCalls returns true if the response contains tool calls
func (r *Response) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// registry holds all registered providers
var registry = make(map[Provider]*ProviderRegistration)

// RegisterProvider registers a new provider
func RegisterProvider(provider Provider, reg ProviderRegistration) {
	registry[provider] = &reg
}

// SupportedProviders returns all registered provider names
func SupportedProviders() []Provider {
	providers := make([]Provider, 0, len(registry))
	for p := range registry {
		providers = append(providers, p)
	}
	return providers
}

// SupportedModels returns models supported by a provider
func SupportedModels(provider Provider) []string {
	if reg, ok := registry[provider]; ok {
		return reg.Models
	}
	return nil
}

// ValidateProviderModel checks if a model is valid for a provider
func ValidateProviderModel(provider Provider, model string) error {
	reg, ok := registry[provider]
	if !ok {
		return fmt.Errorf("unsupported provider: %s", provider)
	}

	if len(reg.Models) == 0 {
		return nil // No validation if models list is empty
	}

	for _, m := range reg.Models {
		if m == model {
			return nil
		}
	}

	return fmt.Errorf("model %s not supported by provider %s", model, provider)
}

// Helper functions for creating messages

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

// ParseToolArguments safely parses tool arguments JSON
func ParseToolArguments(arguments string) (map[string]any, error) {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return map[string]any{}, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(arguments), &parsed); err != nil {
		return nil, fmt.Errorf("invalid tool arguments JSON: %w", err)
	}
	return parsed, nil
}
