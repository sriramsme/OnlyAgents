package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

type AppConfig struct {
	Name          string              `mapstructure:"name"`
	BusBufferSize int                 `mapstructure:"bus_buffer_size"`
	Security      SecurityConfig      `mapstructure:"security"`
	Marketplaces  []MarketplaceConfig `mapstructure:"marketplaces"`
	Memory        Memory              `mapstructure:"memory"`
}

type Memory struct {
	LLM llm.Config `mapstructure:"llm"`
}

type SecurityConfig struct {
	ExecutionMode   string `mapstructure:"execution_mode"`    // restricted | native
	WorkingDir      string `mapstructure:"working_dir"`       // landlock root — all agent exec is jailed here
	AllowNetwork    bool   `mapstructure:"allow_network"`     // maps to network namespace on Linux
	AllowSystemBins bool   `mapstructure:"allow_system_bins"` // expands PATH beyond workdir/bin
}

type MarketplaceConfig struct {
	Name       string                     `mapstructure:"name"`
	Enabled    bool                       `mapstructure:"enabled"`
	URL        string                     `mapstructure:"url"`
	VaultPaths map[string]vault.PathEntry `mapstructure:"vault_paths"`
}

func setAppDefaults(v *viper.Viper) {
	v.SetDefault("name", "OnlyAgents")
	v.SetDefault("bus_buffer_size", 256)
	v.SetDefault("security.execution_mode", "restricted")
	v.SetDefault("security.working_dir", "")
	v.SetDefault("security.allow_network", true)
	v.SetDefault("security.allow_system_bins", true)
}

func LoadAppConfig() (*AppConfig, error) {
	path, err := appConfigPath()
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigFile(path)
	setAppDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if os.IsNotExist(err) {
			// No config file — use defaults
			var cfg AppConfig
			if err := v.Unmarshal(&cfg); err != nil {
				return nil, fmt.Errorf("unmarshal defaults: %w", err)
			}
			return applyDerivedDefaults(&cfg)
		}
		return nil, fmt.Errorf("read app config: %w", err)
	}

	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal app config: %w", err)
	}

	return applyDerivedDefaults(&cfg)
}

func applyDerivedDefaults(cfg *AppConfig) (*AppConfig, error) {
	// Expand ~ in working_dir
	if cfg.Security.WorkingDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		cfg.Security.WorkingDir = filepath.Join(home, ".onlyagents", "workspace")
	} else if strings.HasPrefix(cfg.Security.WorkingDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		cfg.Security.WorkingDir = filepath.Join(home, cfg.Security.WorkingDir[2:])
	}

	// Ensure workspace dir exists
	if err := os.MkdirAll(cfg.Security.WorkingDir, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}

	return cfg, nil
}

func appConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".onlyagents", "config.yaml"), nil
}

func (c *AppConfig) Marketplace(name string) *MarketplaceConfig {
	for i := range c.Marketplaces {
		if c.Marketplaces[i].Name == name {
			return &c.Marketplaces[i]
		}
	}
	return nil
}
