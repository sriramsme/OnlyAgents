package connectors

import (
	"context"
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/config"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// Registry holds all connectors. Lives in kernel.
type Registry struct {
	connectors map[string]Connector
	mu         sync.RWMutex
}

func NewRegistry(
	configDir string,
	vault vault.Vault,
	bus chan<- core.Event,
) (*Registry, error) {
	// Load all connector configs
	configs, err := config.LoadAllConnectorConfigs(configDir)
	if err != nil {
		return nil, fmt.Errorf("load connector configs: %w", err)
	}

	registry := &Registry{
		connectors: make(map[string]Connector),
	}

	// Create each connector
	for name, cfg := range configs {
		if !cfg.Enabled {
			continue
		}

		factory, err := GetFactory(cfg.Platform)
		if err != nil {
			return nil, fmt.Errorf("connector %s: %w", name, err)
		}

		connector, err := factory(cfg.RawConfig, vault, bus)
		if err != nil {
			return nil, fmt.Errorf("connector %s: create: %w", name, err)
		}

		registry.connectors[name] = connector
	}

	return registry, nil
}

func (r *Registry) Register(c Connector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectors[c.Name()] = c
}

func (r *Registry) Get(name string) (Connector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.connectors[name]
	return c, ok
}

func (r *Registry) All() []Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Connector, 0, len(r.connectors))
	for _, c := range r.connectors {
		out = append(out, c)
	}
	return out
}

// GetByCapability returns all connectors that support a capability
// Example: GetByCapability("email") returns all EmailConnector implementations
func (r *Registry) GetByCapability(capability string) []Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Connector
	for _, conn := range r.connectors {
		if SupportsCapability(conn, capability) {
			result = append(result, conn)
		}
	}
	return result
}

// Lifecycle Management

func (r *Registry) ConnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, conn := range r.connectors {
		if err := conn.Connect(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, conn := range r.connectors {
		if err := conn.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, conn := range r.connectors {
		if err := conn.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) DisconnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, conn := range r.connectors {
		if err := conn.Disconnect(ctx); err != nil {
			return err
		}
	}
	return nil
}
