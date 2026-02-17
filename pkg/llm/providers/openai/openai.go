package openai

import (
	"context"
	"fmt"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// const (
// 	openaiAPIBase    = "https://api.openai.com/v1"
// 	openaiMaxRetries = 2
// )

func init() {
	// Register OpenAI provider on package initialization
	llm.RegisterProvider(llm.ProviderOpenAI, llm.ProviderRegistration{
		Models: []string{
			"gpt-5-nano",
		},
		EnvKey: "OPENAI_API_KEY",
		Constructor: func(cfg llm.ProviderConfig) (llm.Client, error) {
			return NewOpenAIClient(cfg)
		},
	})
}

// OpenAIClient implements Client for OpenAI's GPT models
type OpenAIClient struct {
	client      *openai.Client
	model       string
	maxTokens   int
	temperature float64
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(cfg llm.ProviderConfig) (*OpenAIClient, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	apiKey, err := llm.GetAPIKeyFromVault(cfg.Vault, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	// Configure OpenAI client
	config := openai.DefaultConfig(apiKey)

	if cfg.BaseURL != "" {
		config.BaseURL = cfg.BaseURL
	}

	client := openai.NewClientWithConfig(config)

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	temperature := cfg.Temperature
	if temperature == 0 {
		temperature = 0.7 // OpenAI default
	}

	return &OpenAIClient{
		client:      client,
		model:       cfg.Model,
		maxTokens:   maxTokens,
		temperature: temperature,
	}, nil
}

// Chat sends a chat completion request to OpenAI
func (c *OpenAIClient) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	start := time.Now()

	// Convert messages to OpenAI format
	messages := c.toOpenAIMessages(req.Messages)

	// Convert tools to OpenAI format
	tools := c.toOpenAITools(req.Tools)

	logger.Log.Debug("openai request",
		"model", c.model,
		"messages", len(req.Messages),
		"tools", len(req.Tools),
		"max_tokens", c.maxTokens)

	// Build request
	chatReq := openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: messages,
	}

	// Set max tokens
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.maxTokens
	}
	chatReq.MaxCompletionTokens = int(maxTokens)

	// // Set temperature
	// temperature := req.Temperature
	// if temperature == 0 {
	// 	temperature = c.temperature
	// }
	// chatReq.Temperature = float32(temperature)

	// Add tools if provided
	if len(tools) > 0 {
		chatReq.Tools = tools
		// Auto mode: model decides when to use tools
		chatReq.ToolChoice = "auto"
	}

	// Make the request
	chatResp, err := c.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		logger.Log.Error("openai api error",
			"model", c.model,
			"error", err)
		return nil, fmt.Errorf("openai api error: %w", err)
	}

	// Parse response
	resp := c.parseResponse(&chatResp)

	logger.Log.Info("openai response",
		"model", c.model,
		"prompt_tokens", resp.Usage.InputTokens,
		"completion_tokens", resp.Usage.OutputTokens,
		"total_tokens", resp.Usage.TotalTokens,
		"has_tool_calls", resp.HasToolCalls(),
		"tool_call_count", len(resp.ToolCalls),
		"finish_reason", resp.StopReason,
		"latency_ms", time.Since(start).Milliseconds())

	return resp, nil
}

// toOpenAIMessages converts our message format to OpenAI's format
func (c *OpenAIClient) toOpenAIMessages(messages []llm.Message) []openai.ChatCompletionMessage {
	result := make([]openai.ChatCompletionMessage, 0, len(messages))

	for _, msg := range messages {
		oaiMsg := openai.ChatCompletionMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		}

		switch msg.Role {
		case llm.RoleSystem:
			oaiMsg.Role = openai.ChatMessageRoleSystem

		case llm.RoleUser:
			oaiMsg.Role = openai.ChatMessageRoleUser

		case llm.RoleAssistant:
			oaiMsg.Role = openai.ChatMessageRoleAssistant

			// Add tool calls if present
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]openai.ToolCall, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ToolCall{
						ID:   tc.ID,
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					})
				}
				oaiMsg.ToolCalls = toolCalls
			}

		case llm.RoleTool:
			oaiMsg.Role = openai.ChatMessageRoleTool
			oaiMsg.ToolCallID = msg.ToolCallID
			oaiMsg.Name = msg.Name
		}

		result = append(result, oaiMsg)
	}

	return result
}

// toOpenAITools converts our tool format to OpenAI's format
func (c *OpenAIClient) toOpenAITools(tools []llm.ToolDef) []openai.Tool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		tool := openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		}

		result = append(result, tool)
	}

	return result
}

// parseResponse extracts content and tool calls from the response
func (c *OpenAIClient) parseResponse(chatResp *openai.ChatCompletionResponse) *llm.Response {
	if len(chatResp.Choices) == 0 {
		return &llm.Response{
			Content:   "",
			ToolCalls: []llm.ToolCall{},
			Usage: llm.Usage{
				InputTokens:  chatResp.Usage.PromptTokens,
				OutputTokens: chatResp.Usage.CompletionTokens,
				TotalTokens:  chatResp.Usage.TotalTokens,
			},
			Model: chatResp.Model,
		}
	}

	choice := chatResp.Choices[0]
	message := choice.Message

	// Extract tool calls
	var toolCalls []llm.ToolCall
	if len(message.ToolCalls) > 0 {
		toolCalls = make([]llm.ToolCall, 0, len(message.ToolCalls))
		for _, tc := range message.ToolCalls {
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: llm.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	return &llm.Response{
		Content:    message.Content,
		ToolCalls:  toolCalls,
		StopReason: string(choice.FinishReason),
		Usage: llm.Usage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:  chatResp.Usage.TotalTokens,
		},
		Model: chatResp.Model,
	}
}

// Provider returns the provider name
func (c *OpenAIClient) Provider() llm.Provider {
	return llm.ProviderOpenAI
}

// Model returns the model name
func (c *OpenAIClient) Model() string {
	return c.model
}
