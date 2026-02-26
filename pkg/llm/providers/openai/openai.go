package openai

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func init() {
	// Register with all available models
	modelNames := llm.GetSupportedModels(ModelRegistry)

	llm.RegisterProvider(llm.ProviderOpenAI, llm.ProviderRegistration{
		Models: modelNames,
		EnvKey: "OPENAI_API_KEY",
		Constructor: func(cfg llm.ProviderConfig) (llm.Client, error) {
			return NewOpenAIClient(cfg)
		},
	})
}

// OpenAIClient handles both streaming and non-streaming requests
type OpenAIClient struct {
	client       *openai.Client
	model        openai.ChatModel
	capabilities llm.ModelCapabilities

	// Resolved configuration
	maxTokens     int
	temperature   float64
	enableCaching bool
	cacheKey      string
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(cfg llm.ProviderConfig) (*OpenAIClient, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	// Get model capabilities from registry
	caps, err := llm.GetModelCapabilities(cfg.Model, ModelRegistry)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	apiKey, err := llm.GetAPIKeyFromVault(cfg.Vault, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}

	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = caps.DefaultMaxTokens
	}

	temperature := cfg.Temperature
	if temperature == 0 {
		temperature = caps.DefaultTemperature
	}

	// Validate the final configuration
	if maxTokens > caps.MaxTokens {
		return nil, fmt.Errorf("max_tokens %d exceeds model limit %d", maxTokens, caps.MaxTokens)
	}

	if caps.SupportsTemperature {
		if temperature < caps.MinTemperature || temperature > caps.MaxTemperature {
			return nil, fmt.Errorf("temperature %.2f outside valid range [%.2f, %.2f]",
				temperature, caps.MinTemperature, caps.MaxTemperature)
		}
	}

	client := openai.NewClient(opts...)

	return &OpenAIClient{
		client:        &client,
		model:         openai.ChatModel(cfg.Model),
		capabilities:  caps,
		maxTokens:     maxTokens,
		temperature:   temperature,
		enableCaching: caps.SupportsPromptCaching,
		cacheKey:      fmt.Sprintf("agent-%s-%d", cfg.Model, time.Now().Unix()/3600),
	}, nil
}

// Chat sends a non-streaming chat completion request
func (c *OpenAIClient) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	start := time.Now()

	// Runtime validation
	if len(req.Tools) > 0 && !c.capabilities.SupportsToolCalling {
		return nil, fmt.Errorf("model %s does not support tool calling", c.model)
	}

	params := c.buildChatParams(req)

	logger.Log.Debug("openai request",
		"model", c.model,
		"messages", len(req.Messages),
		"tools", len(req.Tools),
		"streaming", false)

	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		logger.Log.Error("openai api error", "model", c.model, "error", err)
		return nil, fmt.Errorf("openai api error: %w", err)
	}

	resp := c.parseResponse(completion)

	logger.Log.Info("openai response",
		"model", c.model,
		"prompt_tokens", resp.Usage.InputTokens,
		"completion_tokens", resp.Usage.OutputTokens,
		// "cached_tokens", resp.Usage.CachedTokens,
		"has_tool_calls", resp.HasToolCalls(),
		"latency_ms", time.Since(start).Milliseconds())

	return resp, nil
}

// StreamChunk represents a chunk of streaming response
type StreamChunk struct {
	Content   string
	ToolCalls []tools.ToolCall
	Done      bool
	Error     error
}

// ChatStream sends a streaming chat completion request
func (c *OpenAIClient) ChatStream(ctx context.Context, req *llm.Request) <-chan StreamChunk {
	ch := make(chan StreamChunk)

	go func() {
		defer close(ch)

		// Capability checks
		if !c.capabilities.SupportsStreaming {
			ch <- StreamChunk{
				Error: fmt.Errorf("model %s does not support streaming", c.model),
				Done:  true,
			}
			return
		}

		if len(req.Tools) > 0 && !c.capabilities.SupportsToolCalling {
			ch <- StreamChunk{
				Error: fmt.Errorf("model %s does not support tool calling", c.model),
				Done:  true,
			}
			return
		}

		start := time.Now()
		params := c.buildChatParams(req)

		logger.Log.Debug("openai streaming request",
			"model", c.model,
			"messages", len(req.Messages),
			"tools", len(req.Tools),
			"streaming", true)

		stream := c.client.Chat.Completions.NewStreaming(ctx, params)
		acc := openai.ChatCompletionAccumulator{}

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			// Check for finished content
			if _, ok := acc.JustFinishedContent(); ok {
				// Content stream finished
				logger.Log.Debug("openai streaming complete",
					"model", c.model,
					"total_tokens", acc.Usage.TotalTokens,
					"latency_ms", time.Since(start).Milliseconds())
			}

			// Check for finished tool call
			if tool, ok := acc.JustFinishedToolCall(); ok {
				// Convert to our format
				tc := tools.ToolCall{
					ID:   tool.ID,
					Type: "function",
					Function: tools.FunctionCall{
						Name:      tool.Name,
						Arguments: tool.Arguments,
					},
				}
				ch <- StreamChunk{ToolCalls: []tools.ToolCall{tc}}
			}

			// Send content delta
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				ch <- StreamChunk{Content: chunk.Choices[0].Delta.Content}
			}
		}

		if err := stream.Err(); err != nil && err != io.EOF {
			logger.Log.Error("stream error", "error", err)
			ch <- StreamChunk{Error: err, Done: true}
		} else {
			ch <- StreamChunk{Done: true}
		}

		logger.Log.Info("openai streaming complete",
			"model", c.model,
			"total_tokens", acc.Usage.TotalTokens,
			"latency_ms", time.Since(start).Milliseconds())
	}()

	return ch
}

// buildChatParams creates the parameters for both streaming and non-streaming
func (c *OpenAIClient) buildChatParams(req *llm.Request) openai.ChatCompletionNewParams {
	messages := c.toOpenAIMessages(req.Messages)
	toolParams := c.toOpenAITools(req.Tools)

	params := openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: messages,
	}

	// Max tokens (capped by model limits)
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.maxTokens
	}
	if maxTokens > c.capabilities.MaxTokens {
		maxTokens = c.capabilities.MaxTokens
	}
	params.MaxCompletionTokens = openai.Int(int64(maxTokens))

	// Temperature (constrained by model)
	if c.capabilities.SupportsTemperature {
		temp := req.Temperature
		if temp == 0 {
			temp = c.temperature
		}
		// Clamp to model's valid range
		if temp < c.capabilities.MinTemperature {
			temp = c.capabilities.MinTemperature
		}
		if temp > c.capabilities.MaxTemperature {
			temp = c.capabilities.MaxTemperature
		}
		params.Temperature = openai.Float(temp)
	}

	// Tools (only if supported)
	if len(toolParams) > 0 && c.capabilities.SupportsToolCalling {
		params.Tools = toolParams
	}

	// Prompt caching (only if supported)
	if c.enableCaching && c.capabilities.SupportsPromptCaching {
		params.PromptCacheKey = openai.String(c.cacheKey)
		params.PromptCacheRetention = openai.ChatCompletionNewParamsPromptCacheRetention("24h")
	}
	return params
}

// toOpenAIMessages converts messages to OpenAI format
func (c *OpenAIClient) toOpenAIMessages(messages []llm.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case llm.RoleSystem:
			result = append(result, openai.SystemMessage(msg.Content))

		case llm.RoleUser:
			result = append(result, openai.UserMessage(msg.Content))

		case llm.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				// Create assistant message with tool calls
				toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {

					toolCall := openai.ChatCompletionMessageFunctionToolCallParam{
						ID:   tc.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}

					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &toolCall})
				}
				assistant := openai.ChatCompletionAssistantMessageParam{}
				assistant.Content.OfString = openai.String(msg.Content)
				assistant.ToolCalls = toolCalls
				result = append(result, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
			} else {
				result = append(result, openai.AssistantMessage(msg.Content))
			}

		case llm.RoleTool:
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		}
	}

	return result
}

// toOpenAITools converts tools to OpenAI format
func (c *OpenAIClient) toOpenAITools(tools []tools.ToolDef) []openai.ChatCompletionToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		tool := openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  openai.FunctionParameters(t.Parameters),
		})
		result = append(result, tool)
	}

	return result
}

// parseResponse extracts content and tool calls
func (c *OpenAIClient) parseResponse(completion *openai.ChatCompletion) *llm.Response {
	if len(completion.Choices) == 0 {
		return &llm.Response{
			Content:   "",
			ToolCalls: []tools.ToolCall{},
			Usage: llm.Usage{
				InputTokens:  int(completion.Usage.PromptTokens),
				OutputTokens: int(completion.Usage.CompletionTokens),
				TotalTokens:  int(completion.Usage.TotalTokens),
				// CachedTokens: int(completion.Usage.PromptTokensCached),
			},
			Model: string(completion.Model),
		}
	}

	choice := completion.Choices[0]
	message := choice.Message

	var toolCalls []tools.ToolCall
	if len(message.ToolCalls) > 0 {
		toolCalls = make([]tools.ToolCall, 0, len(message.ToolCalls))
		for _, tc := range message.ToolCalls {
			toolCalls = append(toolCalls, tools.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: tools.FunctionCall{
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
			InputTokens:  int(completion.Usage.PromptTokens),
			OutputTokens: int(completion.Usage.CompletionTokens),
			TotalTokens:  int(completion.Usage.TotalTokens),
			// CachedTokens: int(completion.Usage.PromptTokensCached),
		},
		Model: string(completion.Model),
	}
}

// Provider returns the provider name
func (c *OpenAIClient) Provider() llm.Provider {
	return llm.ProviderOpenAI
}

// Model returns the model name
func (c *OpenAIClient) Model() string {
	return string(c.model)
}

// SetCaching controls prompt caching
func (c *OpenAIClient) SetCaching(enabled bool) {
	if c.capabilities.SupportsPromptCaching {
		c.enableCaching = enabled
	}
}

// SetCacheKey sets a custom cache key
func (c *OpenAIClient) SetCacheKey(key string) {
	c.cacheKey = key
}

func (c *OpenAIClient) Capabilities() llm.ModelCapabilities {
	return c.capabilities
}
