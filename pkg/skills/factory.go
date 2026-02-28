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

// SkillLoader loads multiple skills from external sources (files, directories, etc.)
type SkillLoader func(ctx context.Context, configDir string, executor interface{}) ([]Skill, error)

var (
	factoryMu sync.RWMutex
	factories = make(map[string]Factory)
	loaders   = make(map[string]SkillLoader) // ex: SKILL.md files
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

// RegisterLoader registers a skill loader (for file-based skills)
func RegisterLoader(name string, loader SkillLoader) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	if loader == nil {
		panic("skills: RegisterLoader loader is nil for " + name)
	}
	loaders[name] = loader
}

// GetLoaders returns all registered loaders
func GetLoaders() map[string]SkillLoader {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	result := make(map[string]SkillLoader, len(loaders))
	for k, v := range loaders {
		result[k] = v
	}
	return result
}
