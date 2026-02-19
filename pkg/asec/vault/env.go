package vault

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	registerProvider(ProviderEnv, func(cfg Config) (Vault, error) {
		return NewEnvVault(cfg)
	})
}

// EnvVault reads secrets from environment variables
type EnvVault struct {
	prefix string // Prefix for environment variables (e.g., "ONLYAGENTS_")
}

// NewEnvVault creates a new environment variable vault
func NewEnvVault(cfg Config) (*EnvVault, error) {

	// .env loading is optional — only for local dev convenience.
	// In production, env vars are set by the runtime directly.
	if cfg.DotEnvPath != "" {
		if err := loadDotEnv(cfg.DotEnvPath); err != nil {
			return nil, fmt.Errorf("dotenv: %w", err)
		}
	}

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

// LoadDotEnv loads environment variables from a .env file
// This should be called before initializing EnvVault
func loadDotEnv(filePath string) error {
	// If no path specified, try default .env
	if filePath == "" {
		filePath = ".env"
	}

	cleanPath := filepath.Clean(filePath)
	file, err := os.Open(cleanPath)
	if err != nil {
		// .env is optional, so don't error if it doesn't exist
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to open .env file: %w", err)
	}

	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("failed to close .env file: %v", err)
		}
	}()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid .env format at line %d: %s", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, "\"'")

		// Set environment variable (don't override existing ones)
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("failed to set env var %s: %w", key, err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading .env file: %w", err)
	}

	return err
}
