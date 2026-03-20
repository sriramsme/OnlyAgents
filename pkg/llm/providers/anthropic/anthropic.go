package anthropic

import (
	"fmt"
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
	modelNames := llm.GetSupportedModels(ModelRegistry)

	llm.RegisterProvider(llm.ProviderAnthropic, llm.ProviderRegistration{
		Models:      modelNames,
		EnvKey:      "ANTHROPIC_API_KEY",
		Constructor: NewAnthropicClient,
	})
}

// AnthropicClient implements llm.Client for Anthropic's Claude
type AnthropicClient struct {
	client       *anthropic.Client
	model        string
	capabilities llm.ModelCapabilities

	// Resolved configuration
	maxTokens     int
	temperature   float64
	enableCaching bool
	cacheKey      string
}

// NewAnthropicClient creates a new Anthropic client
func NewAnthropicClient(cfg llm.ProviderConfig) (llm.Client, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("anthropic: model is required")
	}

	// Get model capabilities from registry
	caps, err := llm.GetModelCapabilities(cfg.Model, ModelRegistry)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithMaxRetries(anthropicMaxRetries),
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = anthropicAPIBase
	}
	opts = append(opts, option.WithBaseURL(baseURL))

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

	client := anthropic.NewClient(opts...)

	return &AnthropicClient{
		client:        &client,
		model:         cfg.Model,
		capabilities:  caps,
		maxTokens:     maxTokens,
		temperature:   temperature,
		enableCaching: caps.SupportsPromptCaching,
		cacheKey:      fmt.Sprintf("agent-%s-%d", cfg.Model, time.Now().Unix()/3600),
	}, nil
}

// Provider returns the provider name
func (c *AnthropicClient) Provider() llm.Provider {
	return llm.ProviderAnthropic
}

// Model returns the model name
func (c *AnthropicClient) Model() string {
	return c.model
}

// SetCaching controls prompt caching
func (c *AnthropicClient) SetCaching(enabled bool) {
	if c.capabilities.SupportsPromptCaching {
		c.enableCaching = enabled
		logger.Log.Debug("anthropic prompt caching", "enabled", enabled)
	}
}

// SetCacheKey sets a custom cache key (for compatibility, not used by Anthropic)
func (c *AnthropicClient) SetCacheKey(key string) {
	c.cacheKey = key
}

// Capabilities returns the model capabilities
func (c *AnthropicClient) Capabilities() llm.ModelCapabilities {
	return c.capabilities
}
