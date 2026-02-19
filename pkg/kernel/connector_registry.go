package kernel

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/config"
)

// NewConnectorRegistry creates a registry and loads all connector configs
func NewConnectorRegistry(
	configDir string,
	vault vault.Vault,
	agentRegistry *AgentRegistry,
) (*ConnectorRegistry, error) {
	// Load all connector configs
	configs, err := config.LoadAllConnectorConfigs(configDir)
	if err != nil {
		return nil, fmt.Errorf("load connector configs: %w", err)
	}

	registry := &ConnectorRegistry{
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

		connector, err := factory(cfg.RawConfig, vault, agentRegistry)
		if err != nil {
			return nil, fmt.Errorf("connector %s: create: %w", name, err)
		}

		registry.connectors[name] = connector
	}

	return registry, nil
}

// Get returns a connector by name
func (r *ConnectorRegistry) Get(name string) (Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	connector, ok := r.connectors[name]
	if !ok {
		return nil, fmt.Errorf("connector not found: %s", name)
	}

	return connector, nil
}

// List returns all connector names
func (r *ConnectorRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.connectors))
	for name := range r.connectors {
		names = append(names, name)
	}
	return names
}

// ConnectAll connects all connectors
func (r *ConnectorRegistry) ConnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, connector := range r.connectors {
		if err := connector.Connect(ctx); err != nil {
			errs = append(errs, fmt.Errorf("connector %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("connect errors: %v", errs)
	}

	return nil
}

// StartAll starts all connectors
func (r *ConnectorRegistry) StartAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, connector := range r.connectors {
		if err := connector.Start(ctx); err != nil {
			errs = append(errs, fmt.Errorf("connector %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("start errors: %v", errs)
	}

	return nil
}

// StopAll stops all connectors
func (r *ConnectorRegistry) StopAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, connector := range r.connectors {
		if err := connector.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("connector %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}

	return nil
}

// DisconnectAll disconnects all connectors
func (r *ConnectorRegistry) DisconnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, connector := range r.connectors {
		if err := connector.Disconnect(ctx); err != nil {
			errs = append(errs, fmt.Errorf("connector %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("disconnect errors: %v", errs)
	}

	return nil
}
