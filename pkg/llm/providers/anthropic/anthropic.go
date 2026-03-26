package anthropic

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
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
	llm.BaseClient
	client *anthropic.Client
	model  string
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

	baseClient, err := llm.NewBaseClient(caps, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
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

	client := anthropic.NewClient(opts...)

	return &AnthropicClient{
		BaseClient: baseClient,
		client:     &client,
		model:      cfg.Model,
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
