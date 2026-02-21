package kernel

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
)

// bootstrap.go
func loadAgents(ctx context.Context, v vault.Vault, configDir string, kernelBus chan<- core.Event) (*agents.Registry, error) {
	registry, err := agents.NewRegistry(configDir, v, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("create agents registry: %w", err)
	}
	return registry, nil
}

func loadConnectors(ctx context.Context, v vault.Vault, configDir string, kernelBus chan<- core.Event) (*connectors.Registry, error) {
	registry, err := connectors.NewRegistry(configDir, v, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("create connector registry: %w", err)
	}
	if err := registry.ConnectAll(ctx); err != nil {
		return nil, fmt.Errorf("connect connectors: %w", err)
	}
	return registry, nil
}

func loadChannels(ctx context.Context, v vault.Vault, configDir string, kernelBus chan<- core.Event) (*channels.Registry, error) {
	// Create connector registry
	registry, err := channels.NewRegistry(configDir, v, kernelBus)

	if err != nil {
		return nil, fmt.Errorf("create channel registry: %w", err)
	}

	// Connect all
	if err := registry.ConnectAll(ctx); err != nil {
		return nil, fmt.Errorf("connect channels: %w", err)
	}

	return registry, nil
}

func loadSkills(ctx context.Context, v vault.Vault, configDir string, kernelBus chan<- core.Event) (*skills.Registry, error) {
	// Create connector registr
	return skills.NewRegistry(), nil
	// registry, err := skills.NewRegistry(configDir, v)
	//
	// if err != nil {
	// 	return nil,fmt.Errorf("create skills registry: %w", err)
	// }
	//
	// // Connect all
	// if err := registry.ConnectAll(ctx); err != nil {
	// 	return nil, fmt.Errorf("connect skills: %w", err)
	// }
	//
	//
	// return registry,nil
}
