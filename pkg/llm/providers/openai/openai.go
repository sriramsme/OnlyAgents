package openai

import (
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
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
