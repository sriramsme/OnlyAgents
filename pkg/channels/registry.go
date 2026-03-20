package channels

import (
	"context"
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// NewConnectorRegistry creates a registry and loads all channel configs
func NewRegistry(
	ctx context.Context,
	vault vault.Vault,
	bus chan<- core.Event,
) (*Registry, error) {
	// Load all channel configs
	configs, err := LoadAllConfigs("")
	if err != nil {
		return nil, fmt.Errorf("load channel configs: %w", err)
	}

	var activePriority int
	registry := &Registry{
		channels: make(map[string]Channel),
	}

	// Create each channel
	for name, cfg := range configs {
		if !cfg.Enabled {
			continue
		}

		factory, err := GetFactory(cfg.Platform)
		if err != nil {
			return nil, fmt.Errorf("channel %s: %w", name, err)
		}

		channel, err := factory(ctx, *cfg, vault, bus)
		if err != nil {
			return nil, fmt.Errorf("channel %s: create: %w", name, err)
		}

		registry.channels[name] = channel

		// Set active channel, keep the highest priority
		if cfg.Priority > activePriority {
			activePriority = cfg.Priority
			registry.active = channel
		}
	}

	return registry, nil
}

func (r *Registry) Register(c Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[c.PlatformName()] = c
}

func (r *Registry) GetActive() *Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return &r.active
}

func (r *Registry) SetActive(c Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = c
}

// Get returns a channel by name
func (r *Registry) Get(name string) (Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	channel, ok := r.channels[name]
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", name)
	}

	return channel, nil
}

// List returns all channel names
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

// ConnectAll connects all channels
// lock released before I/O operations
func (r *Registry) ConnectAll() error {
	// Get snapshot of channels without holding lock during I/O
	r.mu.RLock()
	channels := make([]Channel, 0, len(r.channels))
	names := make([]string, 0, len(r.channels))
	for name, channel := range r.channels {
		channels = append(channels, channel)
		names = append(names, name)
	}
	r.mu.RUnlock()

	// Parallel connect for better performance
	type result struct {
		name string
		err  error
	}

	resultCh := make(chan result, len(channels))
	var wg sync.WaitGroup

	for i, channel := range channels {
		wg.Add(1)
		go func(idx int, ch Channel) {
			defer wg.Done()

			if err := ch.Connect(); err != nil {
				resultCh <- result{name: names[idx], err: err}
			}
		}(i, channel)
	}

	wg.Wait()
	close(resultCh)

	// Collect errors
	var errs []error
	for res := range resultCh {
		errs = append(errs, fmt.Errorf("channel %s: %w", res.name, res.err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("connect errors: %v", errs)
	}
	return nil
}

// StartAll starts all channels
// lock released before I/O operations
func (r *Registry) StartAll() error {
	r.mu.RLock()
	channels := make([]Channel, 0, len(r.channels))
	names := make([]string, 0, len(r.channels))
	for name, channel := range r.channels {
		channels = append(channels, channel)
		names = append(names, name)
	}
	r.mu.RUnlock()

	// Parallel start
	type result struct {
		name string
		err  error
	}

	resultCh := make(chan result, len(channels))
	var wg sync.WaitGroup

	for i, channel := range channels {
		wg.Add(1)
		go func(idx int, ch Channel) {
			defer wg.Done()

			if err := ch.Start(); err != nil {
				resultCh <- result{name: names[idx], err: err}
			}
		}(i, channel)
	}

	wg.Wait()
	close(resultCh)

	var errs []error
	for res := range resultCh {
		errs = append(errs, fmt.Errorf("channel %s: %w", res.name, res.err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("start errors: %v", errs)
	}
	return nil
}

// StopAll stops all channels
func (r *Registry) StopAll() error {
	r.mu.RLock()
	channels := make([]Channel, 0, len(r.channels))
	names := make([]string, 0, len(r.channels))
	for name, channel := range r.channels {
		channels = append(channels, channel)
		names = append(names, name)
	}
	r.mu.RUnlock()

	// Sequential stop (order might matter for cleanup)
	var errs []error
	for i, channel := range channels {
		if err := channel.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", names[i], err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}
	return nil
}

// DisconnectAll disconnects all channels
func (r *Registry) DisconnectAll() error {
	r.mu.RLock()
	channels := make([]Channel, 0, len(r.channels))
	names := make([]string, 0, len(r.channels))
	for name, channel := range r.channels {
		channels = append(channels, channel)
		names = append(names, name)
	}
	r.mu.RUnlock()

	// Sequential disconnect
	var errs []error
	for i, channel := range channels {
		if err := channel.Disconnect(); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", names[i], err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("disconnect errors: %v", errs)
	}
	return nil
}
