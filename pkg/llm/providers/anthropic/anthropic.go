package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

const (
	anthropicAPIBase           = "https://api.anthropic.com"
	anthropicThinkingMinBudget = 1024
	anthropicDefaultBudget     = 2048
	anthropicMaxRetries        = 2
)

func init() {
	// Register Anthropic provider on package initialization
	llm.RegisterProvider(llm.ProviderAnthropic, llm.ProviderRegistration{
		Models: []string{
			"claude-sonnet-4-20250514",
			"claude-sonnet-4-5-20250929",
			"claude-opus-4-5-20251101",
			"claude-opus-4-6",
			"claude-haiku-4-5-20251001",
		},
		EnvKey: "ANTHROPIC_API_KEY",
		Constructor: func(cfg llm.ProviderConfig) (llm.Client, error) {
			return NewAnthropicClient(cfg)
		},
	})
}

// AnthropicClient implements Client for Anthropic's Claude
type AnthropicClient struct {
	client      *anthropic.Client
	model       string
	maxTokens   int
	temperature float64
}

// NewAnthropicClient creates a new Anthropic client
func NewAnthropicClient(cfg llm.ProviderConfig) (*AnthropicClient, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	apiKey, err := llm.GetAPIKeyFromVault(cfg.Vault, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(anthropicMaxRetries),
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = anthropicAPIBase
	}
	opts = append(opts, option.WithBaseURL(baseURL))

	client := anthropic.NewClient(opts...)

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	temperature := cfg.Temperature
	if temperature == 0 {
		temperature = 1.0
	}

	return &AnthropicClient{
		client:      &client,
		model:       cfg.Model,
		maxTokens:   maxTokens,
		temperature: temperature,
	}, nil
}

// Chat sends a chat completion request to Claude
func (c *AnthropicClient) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	start := time.Now()

	// Convert messages to Anthropic format
	systemPrompt, messages, err := c.toAnthropicMessages(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	// Convert tools to Anthropic format
	tools := c.toAnthropicTools(req.Tools)

	// Determine max tokens
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.maxTokens
	}

	// Check if this is a thinking-enabled model
	thinkingEnabled := c.isThinkingModel()

	// Adjust max tokens for thinking models if needed
	if thinkingEnabled && maxTokens <= anthropicThinkingMinBudget {
		logger.Log.Warn("adjusting max_tokens for thinking model",
			"model", c.model,
			"original", maxTokens,
			"adjusted", anthropicDefaultBudget)
		maxTokens = anthropicDefaultBudget
	}

	logger.Log.Debug("anthropic request",
		"model", c.model,
		"messages", len(req.Messages),
		"tools", len(req.Tools),
		"thinking_enabled", thinkingEnabled,
		"max_tokens", maxTokens)

	// Build request params
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		Messages:  messages,
		MaxTokens: int64(maxTokens),
	}

	// Add system prompt if provided
	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{{Text: systemPrompt}}
	}

	// Add tools if provided
	if len(tools) > 0 {
		params.Tools = tools
	}

	// Set temperature (thinking models require temperature = 1)
	temperature := req.Temperature
	if temperature == 0 {
		temperature = c.temperature
	}
	if thinkingEnabled {
		if temperature != 1.0 {
			logger.Log.Debug("overriding temperature for thinking model",
				"requested", temperature,
				"required", 1.0)
		}
		temperature = 1.0 // Required for thinking models
	}
	params.Temperature = anthropic.Float(temperature)

	// Enable extended thinking if applicable
	if thinkingEnabled {
		if budget, ok := c.calculateThinkingBudget(maxTokens); ok {
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
			logger.Log.Debug("enabled extended thinking",
				"budget", budget)
		}
	}

	// Make the request
	message, err := c.client.Messages.New(ctx, params)
	if err != nil {
		logger.Log.Error("anthropic api error",
			"model", c.model,
			"error", err)
		return nil, fmt.Errorf("anthropic api error: %w", err)
	}

	// Parse response
	resp := c.parseResponse(message)

	logger.Log.Info("anthropic response",
		"model", c.model,
		"input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens,
		"total_tokens", resp.Usage.TotalTokens,
		"has_tool_calls", resp.HasToolCalls(),
		"tool_call_count", len(resp.ToolCalls),
		"has_reasoning", resp.ReasoningContent != "",
		"stop_reason", resp.StopReason,
		"latency_ms", time.Since(start).Milliseconds())

	return resp, nil
}

// toAnthropicMessages converts our message format to Anthropic's format
func (c *AnthropicClient) toAnthropicMessages(messages []llm.Message) (string, []anthropic.MessageParam, error) {
	var systemPrompt string
	msgList := make([]anthropic.MessageParam, 0, len(messages))

	// Anthropic requires tool results to be in user messages
	pendingToolResults := make([]anthropic.ContentBlockParamUnion, 0)

	flushPendingToolResults := func() {
		if len(pendingToolResults) == 0 {
			return
		}
		msgList = append(msgList, anthropic.NewUserMessage(pendingToolResults...))
		pendingToolResults = nil
	}

	for _, m := range messages {
		switch m.Role {
		case llm.RoleSystem:
			systemPrompt = m.Content

		case llm.RoleUser:
			flushPendingToolResults()
			msgList = append(msgList, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))

		case llm.RoleAssistant:
			flushPendingToolResults()

			blocks := make([]anthropic.ContentBlockParamUnion, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}

			// Add tool calls
			for _, tc := range m.ToolCalls {
				input := c.parseToolArguments(tc.Function.Arguments)
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					},
				})
			}

			if len(blocks) > 0 {
				msgList = append(msgList, anthropic.NewAssistantMessage(blocks...))
			}

		case llm.RoleTool:
			// Queue tool results to be added to next user message
			pendingToolResults = append(pendingToolResults, anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: m.ToolCallID,
					Content: []anthropic.ToolResultBlockParamContentUnion{{
						OfText: &anthropic.TextBlockParam{Text: m.Content},
					}},
				},
			})

		default:
			return "", nil, fmt.Errorf("unsupported message role: %s", m.Role)
		}
	}

	flushPendingToolResults()
	return systemPrompt, msgList, nil
}

// toAnthropicTools converts our tool format to Anthropic's format
func (c *AnthropicClient) toAnthropicTools(tools []llm.ToolDef) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	result := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		schema := anthropic.ToolInputSchemaParam{
			ExtraFields: map[string]any{},
		}

		if params := t.Function.Parameters; params != nil {
			if properties, ok := params["properties"]; ok {
				schema.Properties = properties
			}
			if required, ok := params["required"]; ok {
				schema.Required = c.normalizeRequired(required)
			}
			// Copy other fields to ExtraFields
			for k, v := range params {
				if k != "type" && k != "properties" && k != "required" {
					schema.ExtraFields[k] = v
				}
			}
		}

		tool := anthropic.ToolParam{
			Name:        t.Function.Name,
			InputSchema: schema,
		}
		if t.Function.Description != "" {
			tool.Description = anthropic.String(t.Function.Description)
		}

		result = append(result, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return result
}

// parseResponse extracts content, reasoning, and tool calls from the response
func (c *AnthropicClient) parseResponse(message *anthropic.Message) *llm.Response {
	var textParts []string
	var reasoningParts []string
	var toolCalls []llm.ToolCall

	for _, block := range message.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "thinking":
			if strings.TrimSpace(block.Thinking) != "" {
				reasoningParts = append(reasoningParts, strings.TrimSpace(block.Thinking))
			}
		case "redacted_thinking":
			reasoningParts = append(reasoningParts, "[redacted_thinking]")
		case "tool_use":
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: llm.FunctionCall{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		}
	}

	content := strings.Join(textParts, "\n")
	reasoningContent := strings.Join(reasoningParts, "\n")

	return &llm.Response{
		Content:          content,
		ReasoningContent: reasoningContent,
		ToolCalls:        toolCalls,
		StopReason:       string(message.StopReason),
		Usage: llm.Usage{
			InputTokens:  int(message.Usage.InputTokens),
			OutputTokens: int(message.Usage.OutputTokens),
			TotalTokens:  int(message.Usage.InputTokens + message.Usage.OutputTokens),
		},
		Model: string(message.Model),
	}
}

// isThinkingModel checks if the current model supports extended thinking
func (c *AnthropicClient) isThinkingModel() bool {
	model := strings.ToLower(c.model)
	return strings.Contains(model, "sonnet-4-5") ||
		strings.Contains(model, "opus-4-6") ||
		strings.Contains(model, "claude-sonnet-4-5") ||
		strings.Contains(model, "claude-opus-4-6")
}

// calculateThinkingBudget calculates the thinking token budget
func (c *AnthropicClient) calculateThinkingBudget(maxTokens int) (int64, bool) {
	if maxTokens <= anthropicThinkingMinBudget {
		return 0, false
	}

	budget := anthropicDefaultBudget
	if budget >= maxTokens {
		budget = maxTokens - 1
	}
	if budget < anthropicThinkingMinBudget {
		return 0, false
	}
	return int64(budget), true
}

// parseToolArguments safely parses tool arguments JSON
func (c *AnthropicClient) parseToolArguments(arguments string) any {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return map[string]any{}
	}

	var parsed any
	if err := json.Unmarshal([]byte(arguments), &parsed); err != nil {
		return arguments // Return as string if parsing fails
	}
	return parsed
}

// normalizeRequired normalizes the required field to []string
func (c *AnthropicClient) normalizeRequired(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// Provider returns the provider name
func (c *AnthropicClient) Provider() llm.Provider {
	return llm.ProviderAnthropic
}

// Model returns the model name
func (c *AnthropicClient) Model() string {
	return c.model
}
