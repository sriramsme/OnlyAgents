// Package gemini provides a Gemini LLM client for OnlyAgents
package gemini

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/genai"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

func init() {
	modelNames := llm.GetSupportedModels(ModelRegistry)

	llm.RegisterProvider(llm.ProviderGemini, llm.ProviderRegistration{
		Models:      modelNames,
		EnvKey:      "GEMINI_API_KEY",
		Constructor: NewGeminiClient,
	})
}

// GeminiClient implements llm.Client for Google Gemini
type GeminiClient struct {
	client       *genai.Client
	model        string
	capabilities llm.ModelCapabilities

	// Resolved configuration
	maxTokens     int
	temperature   float64
	enableCaching bool
	cacheKey      string

	// Caching state
	cachedContentName string           // Name of created cached content
	cacheTTL          time.Duration    // Cache TTL (default 1 hour)
	lastCacheContents []*genai.Content // Track what was cached
	lastCacheTools    []*genai.Tool    // Track cached tools
}

// NewGeminiClient creates a new Gemini client
func NewGeminiClient(cfg llm.ProviderConfig) (llm.Client, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("gemini: model is required")
	}

	// Get model capabilities from registry
	caps, err := llm.GetModelCapabilities(cfg.Model, ModelRegistry)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
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

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to create client: %w", err)
	}

	return &GeminiClient{
		client:        client,
		model:         cfg.Model,
		capabilities:  caps,
		maxTokens:     maxTokens,
		temperature:   temperature,
		enableCaching: false, // caps.SupportsPromptCaching,
		cacheKey:      fmt.Sprintf("agent-%s-%d", cfg.Model, time.Now().Unix()/3600),
		cacheTTL:      1 * time.Hour, // Default 1 hour TTL
	}, nil
}

// Provider returns the provider name
func (c *GeminiClient) Provider() llm.Provider {
	return llm.ProviderGemini
}

// Model returns the model name
func (c *GeminiClient) Model() string {
	return c.model
}

// SetCaching controls context caching
func (c *GeminiClient) SetCaching(enabled bool) {
	if c.capabilities.SupportsPromptCaching {
		c.enableCaching = enabled
		logger.Log.Debug("gemini context caching", "enabled", enabled)
	}
}

// SetCacheKey sets a custom cache key (for compatibility)
func (c *GeminiClient) SetCacheKey(key string) {
	c.cacheKey = key
}

// SetCacheTTL sets the cache TTL duration
func (c *GeminiClient) SetCacheTTL(ttl time.Duration) {
	c.cacheTTL = ttl
	logger.Log.Debug("gemini cache TTL updated", "ttl", ttl)
}

// Capabilities returns the model capabilities
func (c *GeminiClient) Capabilities() llm.ModelCapabilities {
	return c.capabilities
}
