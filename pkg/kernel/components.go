package kernel

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/internal/paths"
	"github.com/sriramsme/OnlyAgents/internal/storage/sqlite"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/conversation"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
	"github.com/sriramsme/OnlyAgents/pkg/message"
	"github.com/sriramsme/OnlyAgents/pkg/notify"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/skills/cli"
	"github.com/sriramsme/OnlyAgents/pkg/skills/marketplace"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

type kernelComponents struct {
	agents                  *agents.Registry
	connectors              *connectors.Registry
	channels                *channels.Registry
	skills                  *skills.Registry
	user                    *config.User
	skillMarketplaceManager *marketplace.Manager
	cliExecutor             *cli.CLIExecutor
	cm                      *conversation.Manager
	mm                      *message.Manager
	memManager              *memory.Manager
	notifier                *notify.Notifier
	workflow                *workflow.Engine
	store                   storage.Storage
	scheduler               *scheduler.Scheduler
}

//nolint:gocyclo
func loadComponents(
	ctx context.Context,
	paths *paths.Paths,
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

	c.user, err = config.LoadUserConfig()
	if err != nil {
		return c, fmt.Errorf("load user config: %w", err)
	}

	c.scheduler = scheduler.New(kernelBus)

	c.cm, err = loadConversationManager(c.store)
	if err != nil {
		return c, fmt.Errorf("load conversation manager: %w", err)
	}

	c.notifier, err = notify.New(c.store, kernelBus, c.user.Identity.Timezone)
	if err != nil {
		return c, fmt.Errorf("load notifier: %w", err)
	}

	c.memManager, err = loadMemoryManager(cfg.Memory, c.store, c.user.Identity.Timezone)
	if err != nil {
		return c, fmt.Errorf("load memory manager: %w", err)
	}

	c.mm, err = loadMessageManager(c.store)
	if err != nil {
		return c, fmt.Errorf("load message manager: %w", err)
	}

	v, err := vault.Load(paths.VaultPath)
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

	c.agents, err = loadAgents(ctx, kernelBus, uiBus, c.cm, c.mm, c.memManager)
	if err != nil {
		return c, fmt.Errorf("load agents: %w", err)
	}
	c.connectors, err = loadConnectors(ctx)
	if err != nil {
		return c, fmt.Errorf("load connectors: %w", err)
	}
	c.channels, err = loadChannels(ctx, v, kernelBus)
	if err != nil {
		return c, fmt.Errorf("load channels: %w", err)
	}
	c.skills, err = loadSkills(cfg.Security)
	if err != nil {
		return c, fmt.Errorf("load skills: %w", err)
	}

	c.workflow = workflow.NewEngine(c.store, kernelBus)
	if err != nil {
		return c, fmt.Errorf("create workflow engine: %w", err)
	}

	return c, nil
}

// loadMemoryManager loads the MemoryManager.
func loadMemoryManager(cfg memory.Config, store storage.Storage, userTZ string) (*memory.Manager, error) {
	mm, err := memory.NewManager(store, cfg, userTZ)
	return mm, err
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
func loadConversationManager(store storage.Storage) (*conversation.Manager, error) {
	cm, err := conversation.New(store)
	if err != nil {
		return nil, fmt.Errorf("create conversation manager: %w", err)
	}
	return cm, nil
}

func loadMessageManager(store storage.Storage) (*message.Manager, error) {
	mm, err := message.New(store)
	if err != nil {
		return nil, fmt.Errorf("create message manager: %w", err)
	}
	return mm, nil
}

// bootstrap.go
func loadAgents(
	ctx context.Context,
	kernelBus chan<- core.Event, uiBus core.UIBus,
	cm *conversation.Manager,
	mm *message.Manager,
	memManager *memory.Manager,
) (*agents.Registry, error) {
	registry, err := agents.NewRegistry(ctx, kernelBus, uiBus, cm, mm, memManager)
	if err != nil {
		return nil, fmt.Errorf("create agents registry: %w", err)
	}
	return registry, nil
}

func loadConnectors(ctx context.Context) (*connectors.Registry, error) {
	registry, err := connectors.NewRegistry(ctx)
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

func loadSkills(security config.SecurityConfig) (*skills.Registry, error) {
	// Create connector registr
	registry, err := skills.NewRegistry("", security)
	if err != nil {
		return nil, fmt.Errorf("create skills registry: %w", err)
	}

	return registry, nil
}
