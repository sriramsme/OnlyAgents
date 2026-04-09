package embedder

import (
	"context"
	"fmt"
	"os"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// Embedder converts text into a vector representation.
// Dimensions() == 0 signals the RecallEngine to skip vector search
// and fall back to FTS.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimensions() int
	Provider() string
}

// Config holds all configuration needed to construct an Embedder.
// Auth resolution follows the same precedence as llm.Config:
//  1. APIKey (direct)
//  2. Vault + APIKeyPath
//  3. APIKeyName (env var name)
//  4. Default provider env var (e.g. OPENAI_API_KEY)
type Config struct {
	Provider string // "openai" | "ollama" | "none"  (default: "none")
	Model    string // optional — provider defaults applied if empty

	// Auth
	APIKey     string
	APIKeyName string // e.g. "MY_OPENAI_KEY"
	Vault      vault.Vault
	APIKeyPath string
	EnvPath    string

	// BaseURL is used as the Ollama endpoint.
	// Default: http://localhost:11434
	BaseURL string
}

// NewEmbedder constructs the appropriate Embedder from cfg.
// Only returns an error for misconfigured non-noop providers
// (e.g. missing API key, unreachable Ollama).
func NewEmbedder(cfg Config) (Embedder, error) {
	switch cfg.Provider {
	case "openai":
		apiKey, err := resolveAPIKey(cfg)
		if err != nil {
			return nil, fmt.Errorf("embedder(openai): %w", err)
		}
		return newOpenAIEmbedder(apiKey, cfg.Model)

	case "ollama":
		return newOllamaEmbedder(cfg.BaseURL, cfg.Model)

	default:
		logger.Log.Info("embedder: no provider configured, semantic search disabled (set provider: openai or ollama to enable)")
		return Noop{}, nil
	}
}

// resolveAPIKey follows the same precedence chain as llm.resolveAPIKey.
func resolveAPIKey(cfg Config) (string, error) {
	// 1. Direct key
	if cfg.APIKey != "" {
		return cfg.APIKey, nil
	}

	// Load .env if path provided
	if cfg.EnvPath != "" {
		if err := vault.LoadDotEnv(cfg.EnvPath); err != nil {
			return "", fmt.Errorf("failed to load .env: %w", err)
		}
	}

	// 2. Vault
	if cfg.Vault != nil && cfg.APIKeyPath != "" {
		return cfg.Vault.GetSecret(context.Background(), cfg.APIKeyPath)
	}

	// 3. Named env var
	if cfg.APIKeyName != "" {
		if val := os.Getenv(cfg.APIKeyName); val != "" {
			return val, nil
		}
	}

	// 4. Provider default env var
	if val := os.Getenv("OPENAI_API_KEY"); val != "" {
		return val, nil
	}

	return "", fmt.Errorf("no API key found: set APIKey, APIKeyName, or OPENAI_API_KEY env var")
}
