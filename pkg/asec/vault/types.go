// Package vault provides unified secrets management for OnlyAgents
package vault

import (
	"context"
	"sync"
	"time"
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

// Config holds vault configuration
type Config struct {
	Type string `mapstructure:"type"` // env, hashicorp, aws, gcp

	// EnvVault config
	Prefix string `mapstructure:"prefix"` // Environment variable prefix

	// HashiCorp Vault config
	Address   string `mapstructure:"address"`
	Token     string `mapstructure:"token"`
	Namespace string `mapstructure:"namespace"`
	MountPath string `mapstructure:"mount_path"` // Default: "secret"

	// AWS Secrets Manager config
	AWSRegion    string `mapstructure:"aws_region"`
	AWSAccessKey string `mapstructure:"aws_access_key"`
	AWSSecretKey string `mapstructure:"aws_secret_key"`

	// GCP Secret Manager config
	GCPProjectID   string `mapstructure:"gcp_project_id"`
	GCPCredentials string `mapstructure:"gcp_credentials"` // Path to credentials file

	// Caching
	EnableCache  bool          `mapstructure:"enable_cache"`
	CacheTTL     time.Duration `mapstructure:"cache_ttl"`      // Default: 5 minutes
	CacheMaxSize int           `mapstructure:"cache_max_size"` // Default: 1000

	// Security
	AuditLog bool `mapstructure:"audit_log"` // Log all secret access
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
