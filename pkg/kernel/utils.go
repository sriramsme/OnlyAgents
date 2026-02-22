package kernel

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
)

// mustLoadVault loads vault config or exits
func loadVault(path string) (vault.Vault, error) {
	v, err := config.LoadVault(path)
	if err != nil {
		return nil, fmt.Errorf("load vault: %w", err)
	}
	return v, nil
}

// bootstrap.go
func loadAgents(ctx context.Context, v vault.Vault, configDir string, kernelBus chan<- core.Event) (*agents.Registry, error) {
	registry, err := agents.NewRegistry(ctx, configDir, v, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("create agents registry: %w", err)
	}
	return registry, nil
}

func loadConnectors(ctx context.Context, v vault.Vault, configDir string, kernelBus chan<- core.Event) (*connectors.Registry, error) {
	registry, err := connectors.NewRegistry(ctx, configDir, v, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("create connector registry: %w", err)
	}
	if err := registry.ConnectAll(); err != nil {
		return nil, fmt.Errorf("connect connectors: %w", err)
	}
	return registry, nil
}

func loadChannels(ctx context.Context, v vault.Vault, configDir string, kernelBus chan<- core.Event) (*channels.Registry, error) {
	// Create connector registry
	registry, err := channels.NewRegistry(ctx, configDir, v, kernelBus)

	if err != nil {
		return nil, fmt.Errorf("create channel registry: %w", err)
	}

	// Connect all
	if err := registry.ConnectAll(); err != nil {
		return nil, fmt.Errorf("connect channels: %w", err)
	}

	return registry, nil
}

func loadSkills(ctx context.Context, configDir string, kernelBus chan<- core.Event) (*skills.Registry, error) {
	// Create connector registr
	registry, err := skills.NewRegistry(ctx, configDir, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("create skills registry: %w", err)
	}

	return registry, nil
}

func (k *Kernel) initializeSkills() error {
	for _, skill := range k.skills.GetAll() {
		if err := skill.Initialize(k.prepareSkillDeps(skill)); err != nil {
			return fmt.Errorf("initialize skill %s: %w", skill.Name(), err)
		}
	}
	return nil
}
func (k *Kernel) prepareSkillDeps(skill skills.Skill) skills.SkillDeps {
	// Get what the skill needs
	requiredCaps := skill.RequiredCapabilities()

	// Find matching connectors
	connectors := make(map[string]any)
	for _, cap := range requiredCaps {
		// Get all connectors that support this capability
		for _, conn := range k.connectors.GetByCapability(cap) {
			connectors[conn.Name()] = conn
		}
	}

	return skills.SkillDeps{
		Outbox:     k.bus,
		Connectors: connectors, // Only relevant connectors
		Config:     nil,
	}
}
