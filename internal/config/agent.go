package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/viper"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// load reads an agent config file into a Config struct.
// It does not validate or attach a vault — callers do that.
func load(configPath string) (*AgentConfig, error) {
	if configPath == "" {
		return nil, fmt.Errorf("config path empty")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("agent config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	setAgentDefaults(v)

	v.SetEnvPrefix("ONLYAGENTS")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg AgentConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Validate skills (skill names should exist in skill registry)
	// Note: This validation happens in kernel after skill registry is created
	// Here we just check format
	if slices.Contains(cfg.Skills, "") {
		return nil, fmt.Errorf("empty skill is not allowed")
	}
	return &cfg, nil
}

// LoadAgentConfig loads a single agent config and attaches the provided vault.
// The vault must already be initialised and validated by the caller (entry point).
func LoadAgentConfig(configPath string, v vault.Vault) (*AgentConfig, error) {
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
func LoadAllAgentsConfig(dir string, v vault.Vault) ([]*AgentConfig, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	var configs []*AgentConfig
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
	v.SetDefault("max_iterations", 10)
	v.SetDefault("max_tool_calls_per_iteration", 3)
	v.SetDefault("max_cumulative_tool_calls", 15)
	v.SetDefault("max_tool_result_tokens", 2000)
	v.SetDefault("max_execution_time", 3*time.Minute)
	v.SetDefault("enable_early_stopping", true)
	v.SetDefault("similar_call_threshold", 3)
}
