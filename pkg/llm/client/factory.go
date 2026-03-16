package client

import (
	"context"
	"fmt"
	"os"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/anthropic"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/gemini"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/openai"
)

type Config struct {
	Provider string
	Model    string

	// authentication sources (choose one)
	APIKey string // direct key value

	APIKeyName string // e.g. "OPENAI_API_KEY"

	Vault   vault.Vault
	KeyPath string

	EnvPath string // optional .env path
	// optional runtime settings
	BaseURL string

	MaxTokens   int
	Temperature float64
}

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
func New(cfg Config) (llm.Client, error) {
	apiKey, err := resolveAPIKey(cfg)
	if err != nil {
		return nil, err
	}

	provider := llm.Provider(cfg.Provider)
	providerCfg := llm.ProviderConfig{
		Model:       cfg.Model,
		APIKey:      apiKey,
		BaseURL:     cfg.BaseURL,
		MaxTokens:   cfg.MaxTokens,
		Temperature: cfg.Temperature,
	}

	switch provider {
	case llm.ProviderOpenAI:
		return openai.NewOpenAIClient(providerCfg)

	case llm.ProviderAnthropic:
		return anthropic.NewAnthropicClient(providerCfg)

	case llm.ProviderGemini:
		return gemini.NewGeminiClient(providerCfg)

	default:
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
}

func resolveAPIKey(cfg Config) (string, error) {
	// 1️⃣ direct key
	if cfg.APIKey != "" {
		return cfg.APIKey, nil
	}

	// 2️⃣ vault lookup
	if cfg.Vault != nil && cfg.KeyPath != "" {
		return cfg.Vault.GetSecret(context.Background(), cfg.KeyPath)
	}

	// load .env if requested
	if cfg.EnvPath != "" {
		err := vault.LoadDotEnv(cfg.EnvPath)
		if err != nil {
			return "", fmt.Errorf("failed to load .env: %w", err)
		}
	}

	// 3️⃣ explicit env variable name
	if cfg.APIKeyName != "" {
		if val := os.Getenv(cfg.APIKeyName); val != "" {
			return val, nil
		}
	}

	// 4️⃣ provider default env key
	envKey := providerEnvKey(llm.Provider(cfg.Provider))
	if envKey != "" {
		if val := os.Getenv(envKey); val != "" {
			return val, nil
		}
	}

	return "", fmt.Errorf("no API key found for provider %s", cfg.Provider)
}

func providerEnvKey(provider llm.Provider) string {
	if key, ok := llm.GetProviderEnvKey(provider); ok {
		return key
	}
	return ""
}
