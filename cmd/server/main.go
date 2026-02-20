package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/api"
	"github.com/sriramsme/OnlyAgents/internal/api/handlers"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/config"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

func main() {
	logger.Initialize("debug", "json")

	fmt.Println("OnlyAgents Server v0.1.0")
	fmt.Println("=========================")

	// Load configurations
	vault := mustLoadVault()
	defer func() {
		if err := vault.Close(); err != nil {
			logger.Log.Error("error closing vault", "error", err)
		}
	}()

	agentConfigs := mustLoadAgentConfigs(vault)
	serverConfig := mustLoadServerConfig()

	// Setup registries
	agentRegistry := mustSetupAgents(agentConfigs, vault)
	defer func() {
		if err := agentRegistry.StopAll(); err != nil {
			logger.Log.Error("error stopping agents", "error", err)
		}
	}()

	connectorRegistry := setupConnectors(vault, agentRegistry, agentConfigs)
	defer shutdownConnectors(connectorRegistry)

	channelRegistry := setupChannels(vault, agentRegistry, agentConfigs)
	defer shutdownChannels(channelRegistry)

	// Start API server
	server := createServer(serverConfig, agentRegistry)
	runServer(server)
}

// mustLoadVault loads vault config or exits
func mustLoadVault() vault.Vault {
	path := getConfigPath(1, "configs/vault.yaml")
	v, err := config.LoadVault(path)
	if err != nil {
		logger.Log.Error("failed to load vault", "error", err)
		os.Exit(1)
	}
	return v
}

// mustLoadAgentConfigs loads agent configs or exits
func mustLoadAgentConfigs(v vault.Vault) []*config.Config {
	path := getConfigPath(2, "configs/agents/")
	configs, err := config.LoadAllAgentsConfig(path, v)
	if err != nil {
		logger.Log.Error("failed to load agent configs", "error", err)
		os.Exit(1)
	}
	return configs
}

// mustLoadServerConfig loads server config or exits
func mustLoadServerConfig() *config.ServerConfig {
	path := getConfigPath(4, "configs/server.yaml")
	cfg, err := config.LoadServerConfig(path)
	if err != nil {
		logger.Log.Error("failed to load server config", "error", err)
		os.Exit(1)
	}
	return cfg
}

// mustSetupAgents creates and starts agent registry or exits
func mustSetupAgents(configs []*config.Config, v vault.Vault) *kernel.AgentRegistry {
	registry, err := kernel.NewAgentRegistry(configs, v)
	if err != nil {
		logger.Log.Error("failed to create agent registry", "error", err)
		os.Exit(1)
	}
	return registry
}

// mustSetupConnectors creates, registers, connects and starts connectors or exits
func setupConnectors(
	v vault.Vault,
	agentRegistry *kernel.AgentRegistry,
	agentConfigs []*config.Config,
) *kernel.ConnectorRegistry {
	path := getConfigPath(3, "configs/connectors/")

	// Create connector registry
	registry, err := kernel.NewConnectorRegistry(path, v, agentRegistry)
	if err != nil {
		logger.Log.Error("failed to create connector registry", "error", err)
		os.Exit(1)
	}

	// Wire connectors to agents
	if err := agentRegistry.RegisterConnectors(agentConfigs, registry); err != nil {
		logger.Log.Error("failed to register connectors", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Connect all
	if err := registry.ConnectAll(ctx); err != nil {
		logger.Log.Error("failed to connect connectors", "error", err)
		os.Exit(1)
	}

	// Start all
	if err := registry.StartAll(ctx); err != nil {
		logger.Log.Error("failed to start connectors", "error", err)
		os.Exit(1)
	}

	return registry
}

func setupChannels(
	v vault.Vault,
	agentRegistry *kernel.AgentRegistry,
	agentConfigs []*config.Config,
) *kernel.ChannelRegistry {
	path := getConfigPath(3, "configs/channels/")

	// Create connector registry
	registry, err := kernel.NewChannelRegistry(path, v, agentRegistry)
	if err != nil {
		logger.Log.Error("failed to create channel registry", "error", err)
		os.Exit(1)
	}

	// Wire connectors to agents
	if err := agentRegistry.RegisterChannels(agentConfigs, registry); err != nil {
		logger.Log.Error("failed to register channels", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Connect all
	if err := registry.ConnectAll(ctx); err != nil {
		logger.Log.Error("failed to connect channels", "error", err)
		os.Exit(1)
	}

	// Start all
	if err := registry.StartAll(ctx); err != nil {
		logger.Log.Error("failed to start channels", "error", err)
		os.Exit(1)
	}

	return registry
}

// shutdownConnectors gracefully shuts down all connectors
func shutdownConnectors(registry *kernel.ConnectorRegistry) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer func() { cancel() }()

	if err := registry.StopAll(ctx); err != nil {
		logger.Log.Error("error stopping connectors", "error", err)
	}
	if err := registry.DisconnectAll(ctx); err != nil {
		logger.Log.Error("error disconnecting connectors", "error", err)
	}
}

// shutdownConnectors gracefully shuts down all connectors
func shutdownChannels(registry *kernel.ChannelRegistry) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer func() { cancel() }()

	if err := registry.StopAll(ctx); err != nil {
		logger.Log.Error("error stopping channels", "error", err)
	}
	if err := registry.DisconnectAll(ctx); err != nil {
		logger.Log.Error("error disconnecting channels", "error", err)
	}
}

// createServer creates and configures the API server
func createServer(cfg *config.ServerConfig, agentRegistry *kernel.AgentRegistry) *api.Server {
	return api.NewServer(
		config.ServerConfig{
			Host:        cfg.Host,
			Port:        cfg.Port,
			APIKeyVault: cfg.APIKeyVault,
			Version:     "0.1.0",
		},
		handlers.Deps{
			Agents:  agentRegistry,
			Version: "0.1.0",
		},
		logger.Log,
	)
}

// runServer starts the server and handles graceful shutdown
func runServer(server *api.Server) {
	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()

	// Wait for interrupt or error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Log.Error("server error", "error", err)
	case sig := <-quit:
		logger.Log.Info("shutdown signal received", "signal", sig)
	}

	// Graceful shutdown
	fmt.Println("\nShutting down...")
	logger.Log.Info("shutdown initiated")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer func() { cancel() }()

	if err := server.Stop(ctx); err != nil {
		logger.Log.Error("server stop error", "error", err)
	}

	logger.Log.Info("shutdown complete")
}

// getConfigPath returns config path from args or default
func getConfigPath(argIndex int, defaultPath string) string {
	if len(os.Args) > argIndex {
		return os.Args[argIndex]
	}
	return defaultPath
}
