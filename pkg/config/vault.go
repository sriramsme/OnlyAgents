package config

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// Load loads config and initializes vault
// This is the ONLY way to load config - vault is always used
func Load(configPath string) (*Config, vault.Vault, error) {
	// Load config from file
	cfg, err := load(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Set vault defaults if not specified
	if cfg.Vault.Type == "" {
		cfg.Vault.Type = "env"
		cfg.Vault.Prefix = "ONLYAGENTS_"
	}

	// Initialize vault (always required)
	v, err := vault.NewVault(cfg.Vault)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize vault: %w", err)
	}

	// Store vault in config for convenient access
	cfg.setVault(v)

	// Validate vault paths exist (with timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := validateVaultPaths(ctx, cfg, v); err != nil {
		if cerr := v.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
		return nil, nil, fmt.Errorf("vault validation failed: %w", err)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		if cerr := v.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
		return nil, nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, v, nil
}

// Close cleans up config resources (especially vault)
func (c *Config) Close() error {
	if c.vault != nil {
		return c.vault.Close()
	}
	return nil
}

// validateVaultPaths checks that all required vault paths exist
// Does NOT fetch the actual secrets (that happens on-demand)
func validateVaultPaths(ctx context.Context, cfg *Config, v vault.Vault) error {
	// Validate LLM API key exists in vault
	if _, err := v.GetSecret(ctx, cfg.LLM.APIKeyVault); err != nil {
		return fmt.Errorf("llm api key not found in vault at '%s': %w", cfg.LLM.APIKeyVault, err)
	}

	// Could add more validation for platform/connector credentials if needed

	return nil
}
