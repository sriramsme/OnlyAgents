// Package vault provides unified secrets management for OnlyAgents
package vault

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// EnvVault reads secrets from environment variables
type EnvVault struct {
	prefix string // Prefix for environment variables (e.g., "ONLYAGENTS_")
}

// NewEnvVault creates a new environment variable vault
func NewEnvVault(cfg Config) (*EnvVault, error) {

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "ONLYAGENTS_" // Default prefix
	}

	// Ensure prefix ends with underscore
	if !strings.HasSuffix(prefix, "_") {
		prefix += "_"
	}

	return &EnvVault{
		prefix: prefix,
	}, nil
}

// GetSecret retrieves a secret from environment variables
// Key format: "llm/anthropic/api_key" -> "ONLYAGENTS_LLM_ANTHROPIC_API_KEY"
func (e *EnvVault) GetSecret(ctx context.Context, key string) (string, error) {
	envKey := e.toEnvKey(key)

	value := os.Getenv(envKey)
	if value == "" {
		// Try without prefix as fallback
		fallbackKey := e.toEnvKeyWithoutPrefix(key)
		value = os.Getenv(fallbackKey)

		if value == "" {
			return "", fmt.Errorf("%w: %s (tried: %s, %s)",
				ErrSecretNotFound, key, envKey, fallbackKey)
		}
	}

	return value, nil
}

// GetSecretWithVersion is not supported for environment variables
func (e *EnvVault) GetSecretWithVersion(ctx context.Context, key, version string) (string, error) {
	// Env vars don't have versions, just return the current value
	return e.GetSecret(ctx, key)
}

// SetSecret sets an environment variable (only for current process)
func (e *EnvVault) SetSecret(ctx context.Context, key, value string) error {
	envKey := e.toEnvKey(key)
	return os.Setenv(envKey, value)
}

// DeleteSecret unsets an environment variable
func (e *EnvVault) DeleteSecret(ctx context.Context, key string) error {
	envKey := e.toEnvKey(key)
	return os.Unsetenv(envKey)
}

// ListSecrets lists all environment variables with the prefix
func (e *EnvVault) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	var secrets []string

	for _, env := range os.Environ() {
		if strings.HasPrefix(env, e.prefix) {
			// Extract key from "KEY=value"
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				// Convert back to vault key format
				envKey := parts[0]
				vaultKey := e.fromEnvKey(envKey)

				if prefix == "" || strings.HasPrefix(vaultKey, prefix) {
					secrets = append(secrets, vaultKey)
				}
			}
		}
	}

	return secrets, nil
}

// Close is a no-op for EnvVault
func (e *EnvVault) Close() error {
	return nil
}

// Name returns the vault type
func (e *EnvVault) Name() string {
	return "env"
}

// toEnvKey converts vault key to environment variable name
// "llm/anthropic/api_key" -> "ONLYAGENTS_LLM_ANTHROPIC_API_KEY"
func (e *EnvVault) toEnvKey(key string) string {
	// Replace slashes with underscores
	envKey := strings.ReplaceAll(key, "/", "_")
	// Replace dashes with underscores
	envKey = strings.ReplaceAll(envKey, "-", "_")
	// Convert to uppercase
	envKey = strings.ToUpper(envKey)
	// Add prefix
	return e.prefix + envKey
}

// toEnvKeyWithoutPrefix converts vault key without prefix
// Used as fallback for standard env vars like OPENAI_API_KEY
func (e *EnvVault) toEnvKeyWithoutPrefix(key string) string {
	envKey := strings.ReplaceAll(key, "/", "_")
	envKey = strings.ReplaceAll(envKey, "-", "_")
	return strings.ToUpper(envKey)
}

// fromEnvKey converts environment variable name back to vault key
// "ONLYAGENTS_LLM_ANTHROPIC_API_KEY" -> "llm/anthropic/api_key"
func (e *EnvVault) fromEnvKey(envKey string) string {
	// Remove prefix
	key := strings.TrimPrefix(envKey, e.prefix)
	// Convert to lowercase
	key = strings.ToLower(key)
	// Replace underscores with slashes
	key = strings.ReplaceAll(key, "_", "/")
	return key
}
