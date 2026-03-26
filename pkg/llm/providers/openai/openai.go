package openai

import (
	"fmt"

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
	llm.BaseClient
	client *openai.Client
	model  openai.ChatModel
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
	baseClient, err := llm.NewBaseClient(caps, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}

	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)

	return &OpenAIClient{
		BaseClient: baseClient,
		client:     &client,
		model:      openai.ChatModel(cfg.Model),
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
