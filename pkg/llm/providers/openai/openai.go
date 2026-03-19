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
		Models:      modelNames,
		EnvKey:      "OPENAI_API_KEY",
		Constructor: NewOpenAIClient,
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
func NewOpenAIClient(cfg llm.ProviderConfig) (llm.Client, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	// Get model capabilities from registry
	caps, err := llm.GetModelCapabilities(cfg.Model, ModelRegistry)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}

	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	maxTokens := caps.DefaultMaxTokens
	temperature := caps.DefaultTemperature

	if cfg.Options != nil {
		if cfg.Options.MaxTokens > 0 {
			maxTokens = cfg.Options.MaxTokens
		}

		if cfg.Options.Temperature > 0 {
			temperature = cfg.Options.Temperature
		}
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

// ChatStream sends a streaming chat completion request
func (c *OpenAIClient) ChatStream(ctx context.Context, req *llm.Request) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)

	go func() {
		defer close(ch)

		// Capability checks
		if !c.capabilities.SupportsStreaming {
			ch <- llm.StreamChunk{
				Error: fmt.Errorf("model %s does not support streaming", c.model),
				Done:  true,
			}
			return
		}

		if len(req.Tools) > 0 && !c.capabilities.SupportsToolCalling {
			ch <- llm.StreamChunk{
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
				ch <- llm.StreamChunk{ToolCalls: []tools.ToolCall{tc}}
			}

			// Send content delta
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				ch <- llm.StreamChunk{Content: chunk.Choices[0].Delta.Content}
			}
		}

		if err := stream.Err(); err != nil && err != io.EOF {
			logger.Log.Error("stream error", "error", err)
			ch <- llm.StreamChunk{Error: err, Done: true}
		} else {
			ch <- llm.StreamChunk{Done: true}
		}

		logger.Log.Info("openai streaming complete",
			"model", c.model,
			"total_tokens", acc.Usage.TotalTokens,
			"latency_ms", time.Since(start).Milliseconds())
	}()

	return ch
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
