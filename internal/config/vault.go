package config

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/viper"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// LoadVault reads configs/vault.yaml, initialises the vault, and returns it.
// The caller (entry point) owns the vault and must call v.Close() on shutdown.
func LoadVault(configPath string) (vault.Vault, error) {
	vc, err := loadVaultConfig(configPath)
	if err != nil {
		return nil, err
	}

	v, err := vault.NewVault(*vc)
	if err != nil {
		return nil, fmt.Errorf("init vault: %w", err)
	}
	return v, nil
}

// loadVaultConfig reads vault.yaml into a vault.Config.
func loadVaultConfig(configPath string) (*vault.Config, error) {
	v := viper.New()

	// sensible defaults so vault.yaml can stay minimal
	v.SetDefault("type", "env")
	v.SetDefault("prefix", "ONLYAGENTS_")
	v.SetDefault("enable_cache", true)
	v.SetDefault("audit_log", false)

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("vault")
		v.SetConfigType("yaml")
		v.AddConfigPath("configs")
		v.AddConfigPath(".")
	}

	v.SetEnvPrefix("ONLYAGENTS")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read vault config: %w", err)
		}
	}

	var vc vault.Config
	if err := v.Unmarshal(&vc); err != nil {
		return nil, fmt.Errorf("unmarshal vault config: %w", err)
	}
	return &vc, nil
}

// LoadVaultAndValidate is a convenience for CLI tools that want to confirm
// vault connectivity and a specific agent config all in one shot.
func LoadVaultAndValidate(vaultPath, agentConfigPath string) (vault.Vault, error) {
	v, err := LoadVault(vaultPath)
	if err != nil {
		return nil, err
	}

	cfg, err := load(agentConfigPath)
	if err != nil {
		cerr := v.Close()
		return nil, fmt.Errorf("load agent config: %w", errors.Join(err, cerr))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := validateVaultPaths(ctx, cfg, v); err != nil {
		cerr := v.Close()
		return nil, fmt.Errorf("vault validation failed: %w", errors.Join(err, cerr))
	}
	return v, nil
}

// validateVaultPaths probes vault to confirm required secrets are reachable.
// It does not return the secret values — that happens on-demand at runtime.
func validateVaultPaths(ctx context.Context, cfg *Config, v vault.Vault) error {
	if _, err := v.GetSecret(ctx, cfg.LLM.APIKeyVault); err != nil {
		return fmt.Errorf("llm api key not found in vault at %q: %w", cfg.LLM.APIKeyVault, err)
	}
	return nil
}
