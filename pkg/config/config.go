package config

import (
	"fmt"
	"os"

	// "path/filepath"

	"github.com/spf13/viper"
)

// Config represents the complete agent configuration
type Config struct {
	Agent     AgentConfig      `mapstructure:"agent"`
	Logging   LoggingConfig    `mapstructure:"logging"`
	Security  SecurityConfig   `mapstructure:"security"`
	LLM       LLMConfig        `mapstructure:"llm"`
	Skills    []SkillConfig    `mapstructure:"skills"`
	Platforms []PlatformConfig `mapstructure:"platforms"`
}

type AgentConfig struct {
	ID             string `mapstructure:"id"`
	Name           string `mapstructure:"name"`
	Role           string `mapstructure:"role"`
	MaxConcurrency int    `mapstructure:"max_concurrency"`
	BufferSize     int    `mapstructure:"buffer_size"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`  // debug, info, warn, error
	Format string `mapstructure:"format"` // text, json
}

type SecurityConfig struct {
	KeyPath         string `mapstructure:"key_path"`
	CredentialsPath string `mapstructure:"credentials_path"`
}

type LLMConfig struct {
	Provider string            `mapstructure:"provider"` // anthropic, openai
	Model    string            `mapstructure:"model"`
	APIKey   string            `mapstructure:"api_key"`
	BaseURL  string            `mapstructure:"base_url"`
	Options  map[string]string `mapstructure:"options"`
}

type SkillConfig struct {
	Name    string            `mapstructure:"name"`
	Enabled bool              `mapstructure:"enabled"`
	Config  map[string]string `mapstructure:"config"`
}

type PlatformConfig struct {
	Name        string            `mapstructure:"name"`
	Type        string            `mapstructure:"type"`
	Enabled     bool              `mapstructure:"enabled"`
	Credentials map[string]string `mapstructure:"credentials"`
}

// Load reads configuration from file
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read from file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Look in default locations
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

	// Load API key from environment if not in config
	if config.LLM.APIKey == "" {
		config.LLM.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	return &config, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("agent.max_concurrency", 10)
	v.SetDefault("agent.buffer_size", 100)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")
	v.SetDefault("llm.provider", "anthropic")
	v.SetDefault("llm.model", "claude-sonnet-4-20250514")
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Agent.ID == "" {
		return fmt.Errorf("agent.id is required")
	}

	if c.LLM.Provider != "" && c.LLM.APIKey == "" {
		return fmt.Errorf("llm.api_key is required when provider is set")
	}

	return nil
}
