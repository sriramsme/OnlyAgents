package vault

import (
	"context"
	"sync"
	"time"
)

type ProviderType string

const (
	ProviderEnv       ProviderType = "env"
	ProviderHashiCorp ProviderType = "hashicorp"
	ProviderAWS       ProviderType = "aws"
	ProviderGCP       ProviderType = "gcp"
)

// Vault is the interface for secrets management
type Vault interface {
	// GetSecret retrieves a secret by key
	GetSecret(ctx context.Context, key string) (string, error)

	// GetSecretWithVersion retrieves a specific version of a secret
	GetSecretWithVersion(ctx context.Context, key, version string) (string, error)

	// SetSecret stores a secret (not all vaults support this)
	SetSecret(ctx context.Context, key, value string) error

	// DeleteSecret removes a secret (not all vaults support this)
	DeleteSecret(ctx context.Context, key string) error

	// ListSecrets lists all secret keys (not all vaults support this)
	ListSecrets(ctx context.Context, prefix string) ([]string, error)

	// Close cleans up resources
	Close() error

	// Name returns the vault type name
	Name() string
}

// VaultPathEntry is shared across channels, connectors, or any resource
// that needs to collect secrets from the user.
type PathEntry struct {
	Path   string `mapstructure:"path"`   // e.g. brave/api_key
	Prompt string `mapstructure:"prompt"` // shown to user
}

// Secret represents a secret with metadata
type Secret struct {
	Key       string
	Value     string
	Version   string
	CreatedAt time.Time
	ExpiresAt *time.Time
	Metadata  map[string]string
}

// Cache entry for secrets
type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// CachedVault wraps a vault with caching
type CachedVault struct {
	vault    Vault
	cache    map[string]cacheEntry
	mu       sync.RWMutex
	ttl      time.Duration
	maxSize  int
	auditLog bool
}
