package kernel

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/internal/bootstrap"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/skills/cli"
	"github.com/sriramsme/OnlyAgents/pkg/skills/marketplace"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
	"github.com/sriramsme/OnlyAgents/pkg/storage/sqlite"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

type AgentInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	IsGeneral    bool     `json:"is_general,omitempty"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

type kernelComponents struct {
	agents                  *agents.Registry
	connectors              *connectors.Registry
	channels                *channels.Registry
	skills                  *skills.Registry
	user                    *config.User
	skillMarketplaceManager *marketplace.Manager
	cliExecutor             *cli.CLIExecutor
	cm                      *memory.ConversationManager
	mm                      *memory.MemoryManager
	workflow                *workflow.Engine
	store                   storage.Storage
}

func loadComponents(
	ctx context.Context,
	paths *bootstrap.Paths,
	cfg *config.AppConfig,
	kernelBus chan core.Event,
	uiBus core.UIBus,
) (kernelComponents, error) {
	var c kernelComponents
	var err error

	c.store, err = loadStore(ctx, paths.DBPath)
	if err != nil {
		return c, fmt.Errorf("load store: %w", err)
	}

	c.cm, err = loadConversationManager(ctx, c.store)
	if err != nil {
		return c, fmt.Errorf("load conversation manager: %w", err)
	}

	c.mm, err = loadMemoryManager(ctx, c.store)
	if err != nil {
		return c, fmt.Errorf("load memory manager: %w", err)
	}

	v, err := loadVault(paths.VaultPath)
	if err != nil {
		return c, fmt.Errorf("load vault: %w", err)
	}

	// 3. Setup marketplace manager
	c.skillMarketplaceManager = marketplace.NewManager(paths.SkillCache, paths.Skills)

	// Register ClawHub marketplace
	clawHub := cfg.Marketplace("clawhub")
	if clawHub == nil {
		logger.Log.Info("loading ClawHub marketplacei failed: marketplace not configured")
	} else if clawHub.Enabled {
		key, err := v.GetSecret(ctx, clawHub.VaultPaths["api_key"].Path)
		if err == nil {
			clawHub := marketplace.NewClawHubMarketplace(
				clawHub.URL,
				key,
			)
			c.skillMarketplaceManager.RegisterMarketplace(clawHub)
		} else {
			logger.Log.Warn("failed to load ClawHub auth token",
				"error", err)
		}
	}

	c.agents, err = loadAgents(ctx, kernelBus, uiBus, c.cm, c.mm)
	if err != nil {
		return c, fmt.Errorf("load agents: %w", err)
	}
	c.connectors, err = loadConnectors(ctx, v, kernelBus)
	if err != nil {
		return c, fmt.Errorf("load connectors: %w", err)
	}
	c.channels, err = loadChannels(ctx, v, kernelBus)
	if err != nil {
		return c, fmt.Errorf("load channels: %w", err)
	}
	c.skills, err = loadSkills()
	if err != nil {
		return c, fmt.Errorf("load skills: %w", err)
	}
	c.user, err = config.LoadUserConfig()
	if err != nil {
		return c, fmt.Errorf("load user config: %w", err)
	}

	c.workflow = workflow.NewEngine(c.store, kernelBus)
	if err != nil {
		return c, fmt.Errorf("create workflow engine: %w", err)
	}

	return c, nil
}

// loadMemoryManager loads the MemoryManager.
func loadMemoryManager(ctx context.Context, store storage.Storage) (*memory.MemoryManager, error) {
	mm := memory.NewMemoryManager(store, nil) // TODO: llmClient for summarizer
	return mm, nil
}

// loadStore loads the SQLite storage.
func loadStore(ctx context.Context, path string) (storage.Storage, error) {
	store, err := sqlite.New(path)
	if err != nil {
		logger.Log.Error("storage init failed", "err", err)
		return nil, fmt.Errorf("storage init failed: %w", err)
	}

	return store, nil
}

// loadConversationManager loads the ConversationManager.
// It is shared by all agents, so they can persist messages and tool results.
func loadConversationManager(ctx context.Context, store storage.Storage) (*memory.ConversationManager, error) {
	cm, err := memory.New(ctx, store)
	if err != nil {
		return nil, fmt.Errorf("create conversation manager: %w", err)
	}
	return cm, nil
}

// mustLoadVault loads vault config or exits
func loadVault(path string) (vault.Vault, error) {
	v, err := vault.LoadVault()
	if err != nil {
		return nil, fmt.Errorf("load vault: %w", err)
	}
	return v, nil
}

// bootstrap.go
func loadAgents(
	ctx context.Context,
	kernelBus chan<- core.Event, uiBus core.UIBus,
	cm *memory.ConversationManager,
	mm *memory.MemoryManager,
) (*agents.Registry, error) {
	registry, err := agents.NewRegistry(ctx, kernelBus, uiBus, cm, mm)
	if err != nil {
		return nil, fmt.Errorf("create agents registry: %w", err)
	}
	return registry, nil
}

func loadConnectors(ctx context.Context, v vault.Vault, kernelBus chan<- core.Event) (*connectors.Registry, error) {
	registry, err := connectors.NewRegistry(ctx, v, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("create connector registry: %w", err)
	}
	if err := registry.ConnectAll(); err != nil {
		return nil, fmt.Errorf("connect connectors: %w", err)
	}
	return registry, nil
}

func loadChannels(
	ctx context.Context, v vault.Vault, kernelBus chan<- core.Event,
) (*channels.Registry, error) {
	// Create connector registry
	registry, err := channels.NewRegistry(ctx, v, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("create channel registry: %w", err)
	}

	// Connect all
	if err := registry.ConnectAll(); err != nil {
		return nil, fmt.Errorf("connect channels: %w", err)
	}

	return registry, nil
}

func loadSkills() (*skills.Registry, error) {
	// Create connector registr
	registry, err := skills.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("create skills registry: %w", err)
	}

	return registry, nil
}
