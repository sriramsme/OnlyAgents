package kernel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
)

type AgentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type kernelComponents struct {
	agents                  *agents.Registry
	connectors              *connectors.Registry
	channels                *channels.Registry
	skills                  *skills.Registry
	user                    *config.UserConfig
	capabilityMap           map[core.Capability][]AgentInfo
	skillMarketplaceManager *marketplace.Manager
	cliExecutor             *cli.CLIExecutor
	capabilities            *core.CapabilityRegistry
	cm                      *memory.ConversationManager
}

func applyConfigDefaults(cfg Config) Config {
	if cfg.BusBufferSize == 0 {
		cfg.BusBufferSize = 256
	}
	if cfg.AgentConfigsDir == "" {
		cfg.AgentConfigsDir = "configs/agents/"
	}
	if cfg.ConnectorConfigsDir == "" {
		cfg.ConnectorConfigsDir = "configs/connectors/"
	}
	if cfg.ChannelConfigsDir == "" {
		cfg.ChannelConfigsDir = "configs/channels/"
	}
	if cfg.SkillConfigsDir == "" {
		cfg.SkillConfigsDir = "configs/skills/"
	}
	if cfg.VaultPath == "" {
		cfg.VaultPath = "configs/vault.yaml"
	}
	return cfg
}

func loadComponents(ctx context.Context, cfg Config, bus chan core.Event) (kernelComponents, error) {
	var c kernelComponents

	store, err := loadStore(ctx, cfg)
	if err != nil {
		return c, fmt.Errorf("load store: %w", err)
	}

	c.cm, err = loadConversationManager(ctx, cfg, store)
	if err != nil {
		return c, fmt.Errorf("load conversation manager: %w", err)
	}

	v, err := loadVault(cfg.VaultPath)
	if err != nil {
		return c, fmt.Errorf("load vault: %w", err)
	}

	c.capabilities = core.NewCapabilityRegistry()

	cliConfig := &cli.ExecutorConfig{
		AllowedShells:    []string{"bash", "sh"},
		MaxOutputSize:    1024 * 1024,
		MaxExecutionTime: 60,
		WorkingDir:       "/tmp",
	}
	c.cliExecutor = cli.NewCLIExecutor(ctx, cliConfig)

	// 3. Setup marketplace manager
	c.skillMarketplaceManager = marketplace.NewManager(cfg.SkillCacheDir, cfg.SkillConfigsDir)

	// Register ClawHub marketplace
	if cfg.ClawHubEnabled {
		key, err := v.GetSecret(ctx, cfg.ClawHubTokenVaultKey)
		if err == nil {
			clawHub := marketplace.NewClawHubMarketplace(
				cfg.ClawHubURL,
				key,
			)
			c.skillMarketplaceManager.RegisterMarketplace(clawHub)
		} else {
			logger.Log.Warn("failed to load ClawHub auth token",
				"error", err)
		}
	}

	c.agents, err = loadAgents(ctx, v, cfg.AgentConfigsDir, bus, c.cm)
	if err != nil {
		return c, fmt.Errorf("load agents: %w", err)
	}
	c.connectors, err = loadConnectors(ctx, v, cfg.ConnectorConfigsDir, bus)
	if err != nil {
		return c, fmt.Errorf("load connectors: %w", err)
	}
	c.channels, err = loadChannels(ctx, v, cfg.ChannelConfigsDir, bus)
	if err != nil {
		return c, fmt.Errorf("load channels: %w", err)
	}
	c.skills, err = loadSkills(ctx, cfg.SkillConfigsDir, bus, c.capabilities, c.cliExecutor)
	if err != nil {
		return c, fmt.Errorf("load skills: %w", err)
	}
	c.user, err = config.LoadUserConfig("configs/user.yaml")
	if err != nil {
		return c, fmt.Errorf("load user config: %w", err)
	}
	c.capabilityMap, err = validateAndBuildCapabilityMap(c.agents, c.skills)
	if err != nil {
		return c, fmt.Errorf("validate agent skills: %w", err)
	}

	return c, nil
}

// loadStore loads the SQLite storage.
func loadStore(ctx context.Context, cfg Config) (storage.Storage, error) {
	store, err := sqlite.New(filepath.Join(os.Getenv("HOME"), ".onlyagents", "onlyagents.db"))
	if err != nil {
		logger.Log.Error("storage init failed", "err", err)
		return nil, fmt.Errorf("storage init failed: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Log.Error("storage close failed", "err", err)
		}
	}()

	return store, nil
}

// loadConversationManager loads the ConversationManager.
// It is shared by all agents, so they can persist messages and tool results.
func loadConversationManager(ctx context.Context, cfg Config, store storage.Storage) (*memory.ConversationManager, error) {
	cm, err := memory.New(ctx, store)
	if err != nil {
		return nil, fmt.Errorf("create conversation manager: %w", err)
	}
	return cm, nil
}

// mustLoadVault loads vault config or exits
func loadVault(path string) (vault.Vault, error) {
	v, err := config.LoadVault(path)
	if err != nil {
		return nil, fmt.Errorf("load vault: %w", err)
	}
	return v, nil
}

// bootstrap.go
func loadAgents(
	ctx context.Context, v vault.Vault,
	configDir string, kernelBus chan<- core.Event,
	cm *memory.ConversationManager,
) (*agents.Registry, error) {
	registry, err := agents.NewRegistry(ctx, configDir, v, kernelBus, cm)
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

func loadSkills(ctx context.Context, configDir string, kernelBus chan<- core.Event,
	capabilityRegistry *core.CapabilityRegistry,
	cliExecutor *cli.CLIExecutor) (*skills.Registry, error) {

	// Create connector registr
	registry, err := skills.NewRegistry(ctx, configDir, kernelBus, capabilityRegistry, cliExecutor)
	if err != nil {
		return nil, fmt.Errorf("create skills registry: %w", err)
	}

	return registry, nil
}
