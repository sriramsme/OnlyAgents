package llm

import (
	"context"
	"fmt"
	"os"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// New creates an LLM client for the given provider configuration.
//
// Authentication is resolved using the following precedence:
//
//  1. APIKey
//     If cfg.APIKey is provided, it is used directly.
//
//  2. Vault + KeyPath
//     If cfg.Vault and cfg.KeyPath are set, the key is fetched from the
//     provided vault implementation.
//
//  3. APIKeyEnvName
//     If cfg.APIKeyEnvName is set, the value of that environment variable
//     will be used as the API key.
//
//  4. Provider default environment variable
//     If none of the above are provided, the SDK falls back to the
//     provider's default environment variable (e.g. OPENAI_API_KEY).
//
// If no API key can be resolved, New returns an error.
//
// Example usage:
//
//	// Direct API key
//	client, err := llm.New(llm.Config{
//	    Provider: llm.ProviderOpenAI,
//	    Model:    "gpt-4o",
//	    APIKey:   "sk-...",
//	})
//
//	// Using environment variable
//	client, err := llm.New(llm.Config{
//	    Provider: llm.ProviderOpenAI,
//	    Model:    "gpt-4o",
//	})
//
//	// Using custom env variable
//	client, err := llm.New(llm.Config{
//	    Provider:      llm.ProviderOpenAI,
//	    Model:         "gpt-4o",
//	    APIKeyEnvName: "MY_OPENAI_KEY",
//	})
//
//	// Using a vault provider
//	client, err := llm.New(llm.Config{
//	    Provider: llm.ProviderOpenAI,
//	    Model:    "gpt-4o",
//	    Vault:    vault,
//	    KeyPath:  "openai/api_key",
//	})
func New(cfg Config) (Client, error) {
	provider := Provider(cfg.Provider)
	reg, ok := registry[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}

	apiKey, err := resolveAPIKey(cfg)
	if err != nil {
		return nil, err
	}

	if err := ValidateProviderModel(provider, cfg.Model); err != nil {
		logger.Log.Warn("model validation failed",
			"provider", cfg.Provider,
			"model", cfg.Model,
			"error", err)
	}

	providerCfg := ProviderConfig{
		Model:   cfg.Model,
		APIKey:  apiKey,
		BaseURL: cfg.BaseURL,
		Options: cfg.Options,
	}

	return reg.Constructor(providerCfg)
}

func ResolveAPIKey(cfg Config) (string, error) {
	return resolveAPIKey(cfg)
}

func resolveAPIKey(cfg Config) (string, error) {
	// 1️⃣ Direct key (highest priority)
	if cfg.APIKey != "" {
		return cfg.APIKey, nil
	}

	// Load .env (setup phase)
	if err := vault.LoadDotEnv(cfg.EnvPath); err != nil {
		return "", fmt.Errorf("failed to load .env: %w", err)
	}

	// 2️⃣ Vault (internal)
	if cfg.Vault != nil && cfg.APIKeyPath != "" {
		return cfg.Vault.GetSecret(context.Background(), cfg.APIKeyPath)
	}

	// 3️⃣ Environment
	envKey := cfg.APIKeyName
	if envKey == "" {
		envKey = providerEnvKey(Provider(cfg.Provider))
	}

	if envKey != "" {
		if val := os.Getenv(envKey); val != "" {
			return val, nil
		}
	}

	return "", fmt.Errorf("no API key found for provider %s", cfg.Provider)
}

func providerEnvKey(provider Provider) string {
	if key, ok := GetProviderEnvKey(provider); ok {
		return key
	}
	return ""
}
