package channels

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/config"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// NewConnectorRegistry creates a registry and loads all connector configs
func NewRegistry(
	configDir string,
	vault vault.Vault,
	bus chan<- core.Event,
) (*Registry, error) {
	// Load all connector configs
	configs, err := config.LoadAllChannelConfigs(configDir)
	if err != nil {
		return nil, fmt.Errorf("load connector configs: %w", err)
	}

	registry := &Registry{
		channels: make(map[string]Channel),
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

		channel, err := factory(cfg.RawConfig, vault, bus)
		if err != nil {
			return nil, fmt.Errorf("channel %s: create: %w", name, err)
		}

		registry.channels[name] = channel
	}

	return registry, nil
}

func (r *Registry) Register(c Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[c.PlatformName()] = c
}

// Get returns a connector by name
func (r *Registry) Get(name string) (Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	channel, ok := r.channels[name]
	if !ok {
		return nil, fmt.Errorf("connector not found: %s", name)
	}

	return channel, nil
}

// List returns all connector names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.channels))
	for name := range r.channels {
		names = append(names, name)
	}
	return names
}

func (r *Registry) All() []Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Channel, 0, len(r.channels))
	for _, c := range r.channels {
		out = append(out, c)
	}
	return out
}

// ConnectAll connects all connectors
func (r *Registry) ConnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, channel := range r.channels {
		if err := channel.Connect(ctx); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("connect errors: %v", errs)
	}

	return nil
}

// StartAll starts all connectors
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, channel := range r.channels {
		if err := channel.Start(ctx); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("start errors: %v", errs)
	}

	return nil
}

// StopAll stops all connectors
func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, channel := range r.channels {
		if err := channel.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}

	return nil
}

// DisconnectAll disconnects all connectors
func (r *Registry) DisconnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, channel := range r.channels {
		if err := channel.Disconnect(ctx); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("disconnect errors: %v", errs)
	}

	return nil
}
