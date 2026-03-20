// pkg/skills/registry.go
package skills

import (
	"context"
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
)

type Registry struct {
	templates map[string]Config // name → config, NOT live instances
	mu        sync.RWMutex
}

func NewRegistry(dir string, security config.SecurityConfig) (*Registry, error) {
	configs, err := LoadAllConfigs(dir)
	if err != nil {
		return nil, fmt.Errorf("load skill configs: %w", err)
	}
	reg := &Registry{
		templates: make(map[string]Config),
	}
	for name, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		cfg.Security = security
		reg.templates[name] = *cfg
	}
	return reg, nil
}

func (r *Registry) Register(cfg Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates[cfg.Name] = cfg
}

// Instantiate creates a live skill instance from a template + connector.
func (r *Registry) Instantiate(
	ctx context.Context,
	name string,
	connector connectors.Connector,
) (Skill, error) {
	cfg, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	factory, err := getFactory(cfg)
	if err != nil {
		return nil, err
	}
	return factory(ctx, cfg, connector)
}

// Get retrieves a skill by name
func (r *Registry) Get(name string) (Config, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.templates[name]
	return s, ok
}

// GetAll returns all registered skills
func (r *Registry) GetAll() []Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Config, 0, len(r.templates))
	for _, s := range r.templates {
		out = append(out, s)
	}
	return out
}

func (r *Registry) ListAll() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.templates))
	for name := range r.templates {
		out = append(out, name)
	}
	return out
}
