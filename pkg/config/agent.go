package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// load reads an agent config file into a Config struct.
// It does not validate or attach a vault — callers do that.
func load(configPath string) (*Config, error) {
	v := viper.New()
	setAgentDefaults(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("agent")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.onlyagents")
		v.AddConfigPath("/etc/onlyagents")
	}

	v.SetEnvPrefix("ONLYAGENTS")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

// LoadAgentConfig loads a single agent config and attaches the provided vault.
// The vault must already be initialised and validated by the caller (entry point).
func LoadAgentConfig(configPath string, v vault.Vault) (*Config, error) {
	cfg, err := load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load agent config: %w", err)
	}
	cfg.setVault(v)
	return cfg, nil
}

// LoadAllAgentsConfig loads every *.yaml under dir, sharing a single vault
// instance across all of them. Returns the configs and the vault so the
// caller owns its lifecycle.
func LoadAllAgentsConfig(dir string, v vault.Vault) ([]*Config, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	var configs []*Config
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}

		cfg, err := load(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", f.Name(), err)
		}

		cfg.setVault(v)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = validateVaultPaths(ctx, cfg, v)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("vault validation %s: %w", f.Name(), err)
		}

		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("config validation %s: %w", f.Name(), err)
		}

		configs = append(configs, cfg)
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no agent configs found in %s", dir)
	}
	return configs, nil
}

func setAgentDefaults(v *viper.Viper) {
	v.SetDefault("agent.max_concurrency", 10)
	v.SetDefault("agent.buffer_size", 100)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")
	v.SetDefault("llm.provider", "anthropic")
	v.SetDefault("llm.model", "claude-sonnet-4-20250514")
	v.SetDefault("vault.type", "env")
	v.SetDefault("vault.prefix", "ONLYAGENTS_")
	v.SetDefault("vault.enable_cache", true)
	v.SetDefault("vault.audit_log", false)
}
