package connectors

import (
	"context"
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// Registry holds all connectors. Lives in kernel.
type Registry struct {
	connectors map[string]Connector
	mu         sync.RWMutex
}

func NewRegistry(
	ctx context.Context,
	vault vault.Vault,
	bus chan<- core.Event,
) (*Registry, error) {
	// Load all connector configs
	configs, err := config.LoadAllConnectorConfigs()
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

		if cfg.Enabled {
			connector, err := factory(ctx, *cfg, vault, bus)
			if err != nil {
				return nil, fmt.Errorf("connector %s: create: %w", name, err)
			}

			registry.connectors[name] = connector
		}
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
// Example: GetByCapability(core.CapabilityEmail) returns all EmailConnector implementations
func (r *Registry) GetByCapability(capability core.Capability) []Connector {
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

// ConnectAll connects all connectors
// lock released before I/O operations
func (r *Registry) ConnectAll() error {
	// Get snapshot of connectors without holding lock during I/O
	r.mu.RLock()
	connectors := make([]Connector, 0, len(r.connectors))
	names := make([]string, 0, len(r.connectors))
	for name, connector := range r.connectors {
		connectors = append(connectors, connector)
		names = append(names, name)
	}
	r.mu.RUnlock()

	// Parallel connect for better performance
	type result struct {
		name string
		err  error
	}

	resultCh := make(chan result, len(connectors))
	var wg sync.WaitGroup

	for i, connector := range connectors {
		wg.Add(1)
		go func(idx int, ch Connector) {
			defer wg.Done()

			if err := ch.Connect(); err != nil {
				resultCh <- result{name: names[idx], err: err}
			}
		}(i, connector)
	}

	wg.Wait()
	close(resultCh)

	// Collect errors
	var errs []error
	for res := range resultCh {
		errs = append(errs, fmt.Errorf("connector %s: %w", res.name, res.err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("connect errors: %v", errs)
	}
	return nil
}

// StartAll starts all connectors
// lock released before I/O operations
func (r *Registry) StartAll() error {
	r.mu.RLock()
	connectors := make([]Connector, 0, len(r.connectors))
	names := make([]string, 0, len(r.connectors))
	for name, connector := range r.connectors {
		connectors = append(connectors, connector)
		names = append(names, name)
	}
	r.mu.RUnlock()

	// Parallel start
	type result struct {
		name string
		err  error
	}

	resultCh := make(chan result, len(connectors))
	var wg sync.WaitGroup

	for i, connector := range connectors {
		wg.Add(1)
		go func(idx int, ch Connector) {
			defer wg.Done()

			if err := ch.Start(); err != nil {
				resultCh <- result{name: names[idx], err: err}
			}
		}(i, connector)
	}

	wg.Wait()
	close(resultCh)

	var errs []error
	for res := range resultCh {
		errs = append(errs, fmt.Errorf("connector %s: %w", res.name, res.err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("start errors: %v", errs)
	}
	return nil
}

// StopAll stops all connectors
func (r *Registry) StopAll() error {
	r.mu.RLock()
	connectors := make([]Connector, 0, len(r.connectors))
	names := make([]string, 0, len(r.connectors))
	for name, connector := range r.connectors {
		connectors = append(connectors, connector)
		names = append(names, name)
	}
	r.mu.RUnlock()

	// Sequential stop (order might matter for cleanup)
	var errs []error
	for i, connector := range connectors {
		if err := connector.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("connector %s: %w", names[i], err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}
	return nil
}

// DisconnectAll disconnects all connectors
func (r *Registry) DisconnectAll() error {
	r.mu.RLock()
	connectors := make([]Connector, 0, len(r.connectors))
	names := make([]string, 0, len(r.connectors))
	for name, connector := range r.connectors {
		connectors = append(connectors, connector)
		names = append(names, name)
	}
	r.mu.RUnlock()

	// Sequential disconnect
	var errs []error
	for i, connector := range connectors {
		if err := connector.Disconnect(); err != nil {
			errs = append(errs, fmt.Errorf("connector %s: %w", names[i], err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("disconnect errors: %v", errs)
	}
	return nil
}
