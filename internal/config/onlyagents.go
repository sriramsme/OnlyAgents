package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// Config is passed to NewKernel to configure it.
type KernelConfig struct {
	BusBufferSize  int    `mapstructure:"bus_buffer_size"`
	DefaultAgentID string `mapstructure:"default_agent_id"`

	ClawHubEnabled       bool   `mapstructure:"clawhub_enabled"`
	ClawHubTokenVaultKey string `mapstructure:"clawhub_token_vault_key"`
	ClawHubURL           string `mapstructure:"clawhub_url"`
}

// LoadKernelConfig reads ~/.onlyagents/config.yaml (or a provided path)
// and returns a Config with defaults applied.
func LoadKernelConfig() (*KernelConfig, error) {

	configPath := OnlyAgentsConfigPath()
	if configPath == "" {
		return nil, fmt.Errorf("config path empty")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("kernel/root config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	// ---- Defaults ----
	v.SetDefault("bus_buffer_size", 256)
	v.SetDefault("default_agent_id", "general")
	v.SetDefault("clawhub_enabled", false)
	v.SetDefault("clawhub_token_vault_key", "")
	v.SetDefault("clawhub_url", "")

	// ---- Env overrides ----
	v.SetEnvPrefix("ONLYAGENTS")
	v.AutomaticEnv()

	// ---- Read config (FAIL if missing) ----
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg KernelConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}
