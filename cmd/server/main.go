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
	ctx, cancel := context.WithCancel(context.Background())

	fmt.Println("OnlyAgents Server v0.1.0")
	fmt.Println("=========================")

	// Load configurations
	vault := mustLoadVault()
	defer func() {
		if err := vault.Close(); err != nil {
			logger.Log.Error("error closing vault", "error", err)
		}
	}()

	serverConfig := mustLoadServerConfig()

	k, err := kernel.NewKernel(kernel.Config{
		BusBufferSize:       100,
		DefaultAgentID:      "default",
		AgentConfigsDir:     "configs/agents/",
		ConnectorConfigsDir: "configs/connectors/",
		ChannelConfigsDir:   "configs/channels/",
		SkillConfigsDir:     "configs/skills/",
	}, vault, ctx, cancel)

	if err != nil {
		logger.Log.Error("failed to initialize kernel", "error", err)
		os.Exit(1) // os.Exit lives HERE, not in the library
	}

	// Start API server
	server := createServer(serverConfig, k)
	runServer(server)
	if err := k.Start(); err != nil {
		logger.Log.Error("failed to start kernel", "error", err)
		os.Exit(1) // os.Exit lives HERE, not in the library
	}

	<-ctx.Done()

	logger.Log.Info("shutting down kernel")
	if err := k.Stop(); err != nil {
		logger.Log.Error("error shutting down kernel", "error", err)
	}
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

// createServer creates and configures the API server
func createServer(cfg *config.ServerConfig, k *kernel.Kernel) *api.Server {
	return api.NewServer(
		config.ServerConfig{
			Host:        cfg.Host,
			Port:        cfg.Port,
			APIKeyVault: cfg.APIKeyVault,
			Version:     "0.1.0",
		},
		handlers.Deps{
			Bus:     k.Bus(),
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
