package llm

import (
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

type Config struct {
	Provider string `mapstructure:"provider"`
	Model    string `mapstructure:"model"`

	// authentication sources (choose one)
	APIKey string // direct key value

	APIKeyName string // e.g. "OPENAI_API_KEY"

	Vault      vault.Vault
	APIKeyPath string `mapstructure:"api_key_path"` // vault path, not the key itself

	EnvPath string // optional .env path
	// optional runtime settings
	BaseURL string   `mapstructure:"base_url"`
	Options *Options `mapstructure:"options,omitempty"`
}

type Options struct {
	MaxTokens    int     `mapstructure:"max_tokens,omitempty"`
	Temperature  float64 `mapstructure:"temperature,omitempty"`
	CacheEnabled bool    `mapstructure:"cache_enabled,omitempty"`
	// CacheKey      string   `mapstructure:"cache_key,omitempty"`
}
