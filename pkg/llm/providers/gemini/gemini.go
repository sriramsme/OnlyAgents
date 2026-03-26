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
	llm.BaseClient
	client *genai.Client
	model  string

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

	baseClient, err := llm.NewBaseClient(caps, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
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
		BaseClient: baseClient,
		client:     client,
		model:      cfg.Model,
		cacheTTL:   1 * time.Hour, // Default 1 hour TTL
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

// SetCacheTTL sets the cache TTL duration
func (c *GeminiClient) SetCacheTTL(ttl time.Duration) {
	c.cacheTTL = ttl
	logger.Log.Debug("gemini cache TTL updated", "ttl", ttl)
}
