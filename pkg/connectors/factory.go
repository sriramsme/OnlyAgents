package connectors

import (
	"context"
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/internal/config"
)

// Factory creates a connector from raw config
type Factory func(
	ctx context.Context,
	cfg config.Connector,
) (Connector, error)

var (
	factoryMu sync.RWMutex
	factories = make(map[string]Factory)
)

// Register registers a connector factory for a platform
// Register wraps typed factory into non-generic Factory
func Register[T Connector](connName string, factory func(context.Context, config.Connector) (T, error)) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	if _, exists := factories[connName]; exists {
		panic("connectors: Register called twice for platform " + connName)
	}
	// Wrap typed factory — T is lost here but type check happens in skill constructor
	factories[connName] = Factory(func(ctx context.Context, cfg config.Connector) (Connector, error) {
		return factory(ctx, cfg)
	})
}

// GetFactory returns the factory for a platform
func GetFactory(platform string) (Factory, error) {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	f, ok := factories[platform]
	if !ok {
		return nil, fmt.Errorf("no factory registered for platform: %s", platform)
	}
	return f, nil
}

// ListRegistered returns all registered platform names
func ListRegistered() []string {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	platforms := make([]string, 0, len(factories))
	for platform := range factories {
		platforms = append(platforms, platform)
	}
	return platforms
}
