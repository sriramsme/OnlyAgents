// Package vault provides unified secrets management for OnlyAgents
package vault

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var providers = map[ProviderType]func(Config) (Vault, error){}

func registerProvider(t ProviderType, fn func(Config) (Vault, error)) {
	providers[t] = fn
}

func NewVault(cfg Config) (Vault, error) {
	// Default to env if not specified
	if cfg.Type == "" {
		cfg.Type = string(ProviderEnv)
	}

	fn, ok := providers[ProviderType(cfg.Type)]
	if !ok {
		return nil, fmt.Errorf("unsupported vault type: %s", cfg.Type)
	}

	v, err := fn(cfg)
	if err != nil {
		return nil, err
	}

	if cfg.EnableCache {
		ttl := cfg.CacheTTL
		if ttl == 0 {
			ttl = 5 * time.Minute
		}
		maxSize := cfg.CacheMaxSize
		if maxSize == 0 {
			maxSize = 1000
		}
		v = &CachedVault{
			vault:    v,
			cache:    make(map[string]cacheEntry),
			ttl:      ttl,
			maxSize:  maxSize,
			auditLog: cfg.AuditLog,
		}
	}

	return v, nil
}

// GetSecret retrieves a secret with caching
func (c *CachedVault) GetSecret(ctx context.Context, key string) (string, error) {
	// Check cache first
	c.mu.RLock()
	if entry, exists := c.cache[key]; exists {
		if time.Now().Before(entry.expiresAt) {
			c.mu.RUnlock()
			if c.auditLog {
				logSecretAccess(key, "cache_hit", c.vault.Name())
			}
			return entry.value, nil
		}
	}
	c.mu.RUnlock()

	// Cache miss or expired, fetch from vault
	value, err := c.vault.GetSecret(ctx, key)
	if err != nil {
		return "", err
	}

	// Store in cache
	c.mu.Lock()
	// Evict oldest if cache is full
	if len(c.cache) >= c.maxSize {
		c.evictOldest()
	}
	c.cache[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	if c.auditLog {
		logSecretAccess(key, "vault_fetch", c.vault.Name())
	}

	return value, nil
}

// GetSecretWithVersion retrieves a specific version (no caching)
func (c *CachedVault) GetSecretWithVersion(ctx context.Context, key, version string) (string, error) {
	return c.vault.GetSecretWithVersion(ctx, key, version)
}

// SetSecret stores a secret and invalidates cache
func (c *CachedVault) SetSecret(ctx context.Context, key, value string) error {
	err := c.vault.SetSecret(ctx, key, value)
	if err != nil {
		return err
	}

	// Invalidate cache
	c.mu.Lock()
	delete(c.cache, key)
	c.mu.Unlock()

	return nil
}

// DeleteSecret removes a secret and invalidates cache
func (c *CachedVault) DeleteSecret(ctx context.Context, key string) error {
	err := c.vault.DeleteSecret(ctx, key)
	if err != nil {
		return err
	}

	// Invalidate cache
	c.mu.Lock()
	delete(c.cache, key)
	c.mu.Unlock()

	return nil
}

// ListSecrets lists secrets (no caching)
func (c *CachedVault) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	return c.vault.ListSecrets(ctx, prefix)
}

// Close cleans up resources
func (c *CachedVault) Close() error {
	c.mu.Lock()
	c.cache = nil
	c.mu.Unlock()
	return c.vault.Close()
}

// Name returns the vault name
func (c *CachedVault) Name() string {
	return c.vault.Name()
}

// evictOldest removes the oldest cache entry
func (c *CachedVault) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.cache {
		if oldestKey == "" || entry.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expiresAt
		}
	}

	if oldestKey != "" {
		delete(c.cache, oldestKey)
	}
}

// ClearCache clears all cached secrets
func (c *CachedVault) ClearCache() {
	c.mu.Lock()
	c.cache = make(map[string]cacheEntry)
	c.mu.Unlock()
}

// Common errors
var (
	ErrSecretNotFound        = errors.New("secret not found")
	ErrSecretVersionNotFound = errors.New("secret version not found")
	ErrOperationNotSupported = errors.New("operation not supported by this vault")
	ErrInvalidConfiguration  = errors.New("invalid vault configuration")
	ErrAuthenticationFailed  = errors.New("vault authentication failed")
)

// Helper function to log secret access (for audit)
func logSecretAccess(key, action, vaultType string) {
	// This would integrate with your logger
	// logger.Log.Info("secret accessed",
	// 	"key", maskSecretKey(key),
	// 	"action", action,
	// 	"vault", vaultType,
	// 	"timestamp", time.Now())
}

// // maskSecretKey masks part of the secret key for logging
// func maskSecretKey(key string) string {
// 	if len(key) <= 8 {
// 		return "****"
// 	}
// 	return key[:4] + "****" + key[len(key)-4:]
// }

// ParseSecretReference parses a secret reference like ${vault:path/to/secret}
func ParseSecretReference(value string) (isRef bool, key string) {
	if len(value) > 9 && value[:8] == "${vault:" && value[len(value)-1] == '}' {
		return true, value[8 : len(value)-1]
	}
	return false, ""
}

// ResolveSecretReference resolves a secret reference
func ResolveSecretReference(ctx context.Context, vault Vault, value string) (string, error) {
	isRef, key := ParseSecretReference(value)
	if !isRef {
		return value, nil
	}

	return vault.GetSecret(ctx, key)
}
