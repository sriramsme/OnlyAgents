package config

import (
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// Config represents the complete agent configuration.
type Config struct {
	Agent      AgentConfig       `mapstructure:"agent"`
	Logging    LoggingConfig     `mapstructure:"logging"`
	Security   SecurityConfig    `mapstructure:"security"`
	LLM        LLMConfig         `mapstructure:"llm"`
	Vault      vault.Config      `mapstructure:"vault"`
	Skills     []SkillConfig     `mapstructure:"skills"`
	Platforms  []PlatformConfig  `mapstructure:"platforms"`
	Connectors []ConnectorConfig `mapstructure:"connectors"`

	// unexported — injected after load, never in yaml
	v vault.Vault
}

type AgentConfig struct {
	ID             string `mapstructure:"id"`
	Name           string `mapstructure:"name"`
	Role           string `mapstructure:"role"`
	MaxConcurrency int    `mapstructure:"max_concurrency"`
	BufferSize     int    `mapstructure:"buffer_size"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type SecurityConfig struct {
	KeyPath         string `mapstructure:"key_path"`
	CredentialsPath string `mapstructure:"credentials_path"`
}

// LLMConfig holds model settings. The actual API key lives in vault.
type LLMConfig struct {
	Provider    string            `mapstructure:"provider"`
	Model       string            `mapstructure:"model"`
	APIKeyVault string            `mapstructure:"api_key_vault"` // vault path, not the key itself
	BaseURL     string            `mapstructure:"base_url"`
	Options     map[string]string `mapstructure:"options"`
}

type SkillConfig struct {
	Name    string         `mapstructure:"name"`
	Enabled bool           `mapstructure:"enabled"`
	Config  map[string]any `mapstructure:"config"`
}

type PlatformConfig struct {
	Name        string         `mapstructure:"name"`
	Type        string         `mapstructure:"type"`
	Enabled     bool           `mapstructure:"enabled"`
	Credentials map[string]any `mapstructure:"credentials"` // vault paths
}

type ConnectorConfig struct {
	Name        string         `mapstructure:"name"`
	Type        string         `mapstructure:"type"`
	Enabled     bool           `mapstructure:"enabled"`
	Credentials map[string]any `mapstructure:"credentials"` // vault paths
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	APIKeyVault  string        `mapstructure:"api_key_vault"` // empty = no auth (local dev)
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
	Version      string        `mapstructure:"version"`
}

// GetVault returns the vault instance attached to this config.
func (c *Config) GetVault() vault.Vault { return c.v }

// setVault attaches a vault instance (used by loaders only).
func (c *Config) setVault(v vault.Vault) { c.v = v }

// Close releases vault resources.
func (c *Config) Close() error {
	if c.v != nil {
		return c.v.Close()
	}
	return nil
}

// Validate checks required fields are present.
func (c *Config) Validate() error {
	if c.Agent.ID == "" {
		return fmt.Errorf("agent.id is required")
	}
	if c.LLM.Provider == "" {
		return fmt.Errorf("llm.provider is required")
	}
	if c.LLM.APIKeyVault == "" {
		return fmt.Errorf("llm.api_key_vault is required (vault path to API key)")
	}
	return nil
}
