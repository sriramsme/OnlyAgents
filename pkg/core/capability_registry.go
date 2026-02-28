package core

import (
	"fmt"
	"sync"
)

// CapabilityRegistry manages dynamic capability registration
type CapabilityRegistry struct {
	capabilities map[Capability]*CapabilityInfo
	mu           sync.RWMutex
}

// CapabilityInfo holds metadata about a capability
type CapabilityInfo struct {
	Name         Capability
	Source       string // "system", "native", "cli", "online"
	RegisteredBy string // skill name that registered it
	Description  string
}

// NewCapabilityRegistry creates a new capability registry
func NewCapabilityRegistry() *CapabilityRegistry {
	reg := &CapabilityRegistry{
		capabilities: make(map[Capability]*CapabilityInfo),
	}

	// Register built-in system capabilities
	systemCaps := []Capability{
		CapabilityEmail,
		CapabilityCalendar,
		CapabilityWebSearch,
		CapabilityWebFetch,
		CapabilityTasks,
		CapabilityStorage,
		CapabilityNotes,
		CapabilitySMS,
	}

	for _, cap := range systemCaps {
		reg.capabilities[cap] = &CapabilityInfo{
			Name:   cap,
			Source: "system",
		}
	}

	return reg
}

// Register registers a new capability
func (r *CapabilityRegistry) Register(cap Capability, info *CapabilityInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.capabilities[cap]; exists {
		// Already registered - that's okay, multiple skills can provide same capability
		return nil
	}

	r.capabilities[cap] = info
	return nil
}

// Has checks if a capability is registered
func (r *CapabilityRegistry) Has(cap Capability) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.capabilities[cap]
	return exists
}

// Get returns capability info
func (r *CapabilityRegistry) Get(cap Capability) (*CapabilityInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.capabilities[cap]
	if !exists {
		return nil, fmt.Errorf("capability not found: %s", cap)
	}

	return info, nil
}

// List returns all registered capabilities
func (r *CapabilityRegistry) List() []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	caps := make([]Capability, 0, len(r.capabilities))
	for cap := range r.capabilities {
		caps = append(caps, cap)
	}
	return caps
}

// ListBySource returns capabilities from a specific source
func (r *CapabilityRegistry) ListBySource(source string) []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	caps := make([]Capability, 0)
	for cap, info := range r.capabilities {
		if info.Source == source {
			caps = append(caps, cap)
		}
	}
	return caps
}

// ListAll returns all capabilities
func (r *CapabilityRegistry) ListAll() []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	caps := make([]Capability, 0, len(r.capabilities))
	for cap := range r.capabilities {
		caps = append(caps, cap)
	}
	return caps
}
