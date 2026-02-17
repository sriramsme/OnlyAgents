package llm

import (
	"context"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// getAPIKeyFromVault is an internal helper to fetch API keys from vault
// Used during client initialization only
func GetAPIKeyFromVault(v vault.Vault, keyPath string) (string, error) {
	if v == nil {
		return "", fmt.Errorf("vault is required")
	}

	if keyPath == "" {
		return "", fmt.Errorf("vault key path is required")
	}

	// Create context with timeout for vault operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	apiKey, err := v.GetSecret(ctx, keyPath)
	if err != nil {
		return "", fmt.Errorf("failed to get API key from vault at '%s': %w", keyPath, err)
	}

	if apiKey == "" {
		return "", fmt.Errorf("API key is empty at vault path '%s'", keyPath)
	}

	return apiKey, nil
}
