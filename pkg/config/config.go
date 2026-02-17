package config

import (
	"fmt"

	"github.com/spf13/viper"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// Config represents the complete agent configuration
type Config struct {
	Agent      AgentConfig       `mapstructure:"agent"`
	Logging    LoggingConfig     `mapstructure:"logging"`
	Security   SecurityConfig    `mapstructure:"security"`
	LLM        LLMConfig         `mapstructure:"llm"`
	Skills     []SkillConfig     `mapstructure:"skills"`
	Platforms  []PlatformConfig  `mapstructure:"platforms"`
	Vault      vault.Config      `mapstructure:"vault"`
	Connectors []ConnectorConfig `mapstructure:"connectors"`

	// Vault instance - not exported, not in config file
	vault vault.Vault
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

// LLMConfig - NO API key stored, only vault path
type LLMConfig struct {
	Provider    string            `mapstructure:"provider"`
	Model       string            `mapstructure:"model"`
	APIKeyVault string            `mapstructure:"api_key_vault"` // Vault path only
	BaseURL     string            `mapstructure:"base_url"`
	Options     map[string]string `mapstructure:"options"`
}

type SkillConfig struct {
	Name    string         `mapstructure:"name"`
	Enabled bool           `mapstructure:"enabled"`
	Config  map[string]any `mapstructure:"config"` // Vault paths, not actual values
}

type PlatformConfig struct {
	Name        string         `mapstructure:"name"`
	Type        string         `mapstructure:"type"`
	Enabled     bool           `mapstructure:"enabled"`
	Credentials map[string]any `mapstructure:"credentials"` // Vault paths
}

type ConnectorConfig struct {
	Name        string         `mapstructure:"name"`
	Type        string         `mapstructure:"type"`
	Enabled     bool           `mapstructure:"enabled"`
	Credentials map[string]any `mapstructure:"credentials"` // Vault paths
}

// GetVault returns the vault instance
func (c *Config) GetVault() vault.Vault {
	return c.vault
}

// setVault sets the vault instance (internal use only)
func (c *Config) setVault(v vault.Vault) {
	c.vault = v
}

// load reads configuration from file (private - only called by Load)
func load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read from file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("agent")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.onlyagents")
		v.AddConfigPath("/etc/onlyagents")
	}

	// Read environment variables
	v.SetEnvPrefix("ONLYAGENTS")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &config, nil
}

func setDefaults(v *viper.Viper) {
	// Agent defaults
	v.SetDefault("agent.max_concurrency", 10)
	v.SetDefault("agent.buffer_size", 100)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")

	// LLM defaults
	v.SetDefault("llm.provider", "anthropic")
	v.SetDefault("llm.model", "claude-sonnet-4-20250514")

	// Vault defaults - ALWAYS use vault
	v.SetDefault("vault.type", "env")
	v.SetDefault("vault.prefix", "ONLYAGENTS_")
	v.SetDefault("vault.enable_cache", true)
	v.SetDefault("vault.audit_log", false)
}

// Validate checks if the configuration is valid
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
