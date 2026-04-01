package llm

import (
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

type Config struct {
	Provider string `mapstructure:"provider" json:"provider"`
	Model    string `mapstructure:"model" json:"model"`

	// authentication sources (choose one)
	APIKey string `mapstructure:"api_key" json:"-"` // direct key value

	APIKeyName string `json:"api_key_name,omitempty"` // e.g. "OPENAI_API_KEY"

	Vault      vault.Vault
	APIKeyPath string `mapstructure:"api_key_path" json:"api_key_path,omitempty"` // vault path

	EnvPath string `json:"env_path,omitempty"` // optional .env path

	// optional runtime settings
	BaseURL string   `mapstructure:"base_url" json:"base_url,omitempty"`
	Options *Options `mapstructure:"options,omitempty" json:"options,omitempty"`
}

type Options struct {
	MaxTokens    int     `mapstructure:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	Temperature  float64 `mapstructure:"temperature,omitempty" json:"temperature,omitempty"`
	CacheEnabled bool    `mapstructure:"cache_enabled,omitempty" json:"cache_enabled,omitempty"`
	// CacheKey string `mapstructure:"cache_key,omitempty" json:"cache_key,omitempty"`
}
