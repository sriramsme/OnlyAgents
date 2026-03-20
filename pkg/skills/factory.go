package skills

import (
	"context"
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
)

// Factory creates a skill from a config.
type Factory func(
	ctx context.Context,
	cfg Config,
	conn connectors.Connector,
) (Skill, error)

var (
	factoryMu sync.RWMutex
	factories = make(map[string]Factory)
)

// Register registers a factory.
// Native skills register by name (e.g. "summarize").
// The CLI loader registers as "cli".
func Register(key string, factory Factory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	if factory == nil {
		panic("skills: Register factory is nil for " + key)
	}
	if _, exists := factories[key]; exists {
		panic("skills: Register called twice for " + key)
	}
	factories[key] = factory
}

func getFactory(cfg Config) (Factory, error) {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	switch cfg.Type {
	case "cli":
		f, ok := factories["cli"]
		if !ok {
			return nil, fmt.Errorf("cli skill factory not registered")
		}
		return f, nil
	case "native":
		f, ok := factories[cfg.Name]
		if !ok {
			return nil, fmt.Errorf("no native factory registered for skill %q", cfg.Name)
		}
		return f, nil
	default:
		return nil, fmt.Errorf("unknown skill type %q in skill %q", cfg.Type, cfg.Name)
	}
}
