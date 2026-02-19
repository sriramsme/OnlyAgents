//go:build vault_hashicorp

package providers

import (
	"context"
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
)

func init() {
	registerProvider(ProviderHashiCorp, func(cfg Config) (Vault, error) {
		return NewHashiCorpVault(cfg)
	})
}

// HashiCorpVault implements Vault interface for HashiCorp Vault
type HashiCorpVault struct {
	client    *vaultapi.Client
	mountPath string // KV secrets engine mount path (default: "secret")
}

// NewHashiCorpVault creates a new HashiCorp Vault client
func NewHashiCorpVault(cfg Config) (*HashiCorpVault, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("%w: address is required", ErrInvalidConfiguration)
	}

	if cfg.Token == "" {
		return nil, fmt.Errorf("%w: token is required", ErrInvalidConfiguration)
	}

	// Create vault client configuration
	vaultCfg := vaultapi.DefaultConfig()
	vaultCfg.Address = cfg.Address

	client, err := vaultapi.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Set token
	client.SetToken(cfg.Token)

	// Set namespace if provided
	if cfg.Namespace != "" {
		client.SetNamespace(cfg.Namespace)
	}

	mountPath := cfg.MountPath
	if mountPath == "" {
		mountPath = "secret" // Default KV v2 mount path
	}

	return &HashiCorpVault{
		client:    client,
		mountPath: mountPath,
	}, nil
}

// GetSecret retrieves a secret from HashiCorp Vault
// Key format: "llm/anthropic/api_key" -> secret/data/llm/anthropic (field: api_key)
func (h *HashiCorpVault) GetSecret(ctx context.Context, key string) (string, error) {
	path, field := h.parseKey(key)

	// Read secret from KV v2
	secret, err := h.client.KVv2(h.mountPath).Get(ctx, path)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, key)
	}

	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, key)
	}

	// Get the specific field
	value, ok := secret.Data[field]
	if !ok {
		return "", fmt.Errorf("%w: field %s not found in %s", ErrSecretNotFound, field, path)
	}

	valueStr, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("secret value is not a string: %s", key)
	}

	return valueStr, nil
}

// GetSecretWithVersion retrieves a specific version of a secret
func (h *HashiCorpVault) GetSecretWithVersion(ctx context.Context, key, version string) (string, error) {
	path, field := h.parseKey(key)

	// Parse version
	var versionNum int
	_, err := fmt.Sscanf(version, "%d", &versionNum)
	if err != nil {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Read specific version from KV v2
	secret, err := h.client.KVv2(h.mountPath).GetVersion(ctx, path, versionNum)
	if err != nil {
		return "", fmt.Errorf("%w: %s version %s", ErrSecretVersionNotFound, key, version)
	}

	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("%w: %s version %s", ErrSecretVersionNotFound, key, version)
	}

	value, ok := secret.Data[field]
	if !ok {
		return "", fmt.Errorf("%w: field %s not found in %s", ErrSecretNotFound, field, path)
	}

	valueStr, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("secret value is not a string: %s", key)
	}

	return valueStr, nil
}

// SetSecret stores a secret in HashiCorp Vault
func (h *HashiCorpVault) SetSecret(ctx context.Context, key, value string) error {
	path, field := h.parseKey(key)

	// Read existing secret to preserve other fields
	existing, _ := h.client.KVv2(h.mountPath).Get(ctx, path)

	data := make(map[string]interface{})
	if existing != nil && existing.Data != nil {
		data = existing.Data
	}

	// Update the field
	data[field] = value

	// Write secret to KV v2
	_, err := h.client.KVv2(h.mountPath).Put(ctx, path, data)
	if err != nil {
		return fmt.Errorf("failed to write secret: %w", err)
	}

	return nil
}

// DeleteSecret removes a secret from HashiCorp Vault
func (h *HashiCorpVault) DeleteSecret(ctx context.Context, key string) error {
	path, _ := h.parseKey(key)

	// Delete latest version
	err := h.client.KVv2(h.mountPath).DeleteMetadata(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// ListSecrets lists secrets in HashiCorp Vault
func (h *HashiCorpVault) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	secret, err := h.client.Logical().List(fmt.Sprintf("%s/metadata/%s", h.mountPath, prefix))
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}

	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return []string{}, nil
	}

	var secrets []string
	for _, key := range keys {
		if keyStr, ok := key.(string); ok {
			secrets = append(secrets, prefix+keyStr)
		}
	}

	return secrets, nil
}

// Close cleans up resources
func (h *HashiCorpVault) Close() error {
	// Clear token
	h.client.ClearToken()
	return nil
}

// Name returns the vault type
func (h *HashiCorpVault) Name() string {
	return "hashicorp"
}

// parseKey splits a key into path and field
// "llm/anthropic/api_key" -> path: "llm/anthropic", field: "api_key"
func (h *HashiCorpVault) parseKey(key string) (path string, field string) {
	// Find last slash
	lastSlash := -1
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			lastSlash = i
			break
		}
	}

	if lastSlash == -1 {
		// No slash, entire key is the field
		return "", key
	}

	return key[:lastSlash], key[lastSlash+1:]
}

// Example usage:
//
// vault, _ := NewHashiCorpVault(Config{
//     Address:   "https://vault.example.com",
//     Token:     "s.abc123...",
//     MountPath: "secret",
// })
//
// // Store secret
// vault.SetSecret(ctx, "llm/anthropic/api_key", "sk-ant-...")
//
// // Get secret
// key, _ := vault.GetSecret(ctx, "llm/anthropic/api_key")
//
// // Get specific version
// oldKey, _ := vault.GetSecretWithVersion(ctx, "llm/anthropic/api_key", "3")
//
// // List secrets
// secrets, _ := vault.ListSecrets(ctx, "llm/")
