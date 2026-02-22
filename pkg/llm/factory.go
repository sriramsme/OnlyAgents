// Package llm provides LLM client abstractions for OnlyAgents
package llm

import (
	"fmt"
	"strconv"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// Factory creates LLM clients from configuration
type Factory struct {
	config *config.Config
	vault  vault.Vault
}

// NewFactory creates a new LLM client factory
func NewFactory(cfg *config.Config, vault vault.Vault) *Factory {
	return &Factory{config: cfg, vault: vault}
}

// Create creates an LLM client using the default provider from config
func (f *Factory) Create() (Client, error) {
	return f.CreateForProvider(Provider(f.config.LLM.Provider), f.config.LLM.Model)
}

// CreateForProvider creates an LLM client for a specific provider and model
// This is useful for multi-agent scenarios where different agents use different LLMs
func (f *Factory) CreateForProvider(provider Provider, model string) (Client, error) {
	reg, ok := registry[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	// Validate model
	if err := ValidateProviderModel(provider, model); err != nil {
		logger.Log.Warn("model validation failed",
			"provider", provider,
			"model", model,
			"error", err)
	}

	// Build provider config
	providerCfg := ProviderConfig{
		Model:       model,
		Vault:       f.vault,
		KeyPath:     f.config.LLM.APIKeyVault,
		BaseURL:     f.config.LLM.BaseURL,
		MaxTokens:   f.getMaxTokens(),
		Temperature: f.getTemperature(),
		Metadata:    f.config.LLM.Options,
	}

	// Create client
	client, err := reg.Constructor(providerCfg)
	if err != nil {
		logger.Log.Error("failed to create LLM client",
			"provider", provider,
			"model", model,
			"error", err)
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	logger.Log.Info("created LLM client",
		"provider", provider,
		"model", model)

	return client, nil
}

// getMaxTokens gets max tokens from config options
func (f *Factory) getMaxTokens() int {
	if f.config.LLM.Options == nil {
		return 4096 // default
	}

	if val, ok := f.config.LLM.Options["max_tokens"]; ok {
		if tokens, err := strconv.Atoi(val); err == nil {
			return tokens
		}
	}

	return 4096 // default
}

// getTemperature gets temperature from config options
func (f *Factory) getTemperature() float64 {
	if f.config.LLM.Options == nil {
		return 1.0 // default
	}

	if val, ok := f.config.LLM.Options["temperature"]; ok {
		if temp, err := strconv.ParseFloat(val, 64); err == nil {
			return temp
		}
	}

	return 1.0 // default
}
