package skills

import (
	"context"
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// Factory creates a connector from raw config
type Factory func(
	ctx context.Context,
	eventBus chan<- core.Event,
) (Skill, error)

var (
	factoryMu sync.RWMutex
	factories = make(map[string]Factory)
)

// Register registers a connector factory for a platform
func Register(platform string, factory Factory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()

	if factory == nil {
		panic("skills: Register factory is nil for platform " + platform)
	}
	if _, exists := factories[platform]; exists {
		panic("skillss: Register called twice for platform " + platform)
	}

	factories[platform] = factory
}

// GetFactory returns the factory for a platform
func GetFactory(platform string) (Factory, error) {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	factory, ok := factories[platform]
	if !ok {
		return nil, fmt.Errorf("no factory registered for platform: %s", platform)
	}

	return factory, nil
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
