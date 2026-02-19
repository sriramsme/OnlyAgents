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
	"github.com/sriramsme/OnlyAgents/pkg/config"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap" // auto-registers all providers
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

func main() {

	logger.Initialize("debug", "json")

	fmt.Println("OnlyAgents Server v0.1.0")
	fmt.Println("=========================")
	fmt.Println()

	// load vault
	vaultConfigPath := "configs/vault.yaml"
	if len(os.Args) > 1 {
		vaultConfigPath = os.Args[1]
	}

	vault, err := config.LoadVault(vaultConfigPath)
	if err != nil {
		fmt.Printf("load vault: %v\n", err)
	}

	// Config path from args or default
	agentConfigsPath := "configs/agents/"
	if len(os.Args) > 2 {
		agentConfigsPath = os.Args[2]
	}

	configs, err := config.LoadAllAgentsConfig(agentConfigsPath, vault)
	if err != nil {
		fmt.Printf("load agents: %v\n", err)
	}

	defer func() {
		if err := vault.Close(); err != nil {
			logger.Log.Error("error closing vault", "error", err)
		}
	}()

	registry, err := kernel.NewAgentRegistry(configs, vault)
	if err != nil {
		fmt.Printf("create agent registry: %v\n", err)
	}
	defer func() {
		if err := registry.StopAll(); err != nil {
			logger.Log.Error("error stopping agents", "error", err)
		}
	}()

	// load server config
	serverConfigPath := "configs/server.yaml"
	if len(os.Args) > 3 {
		serverConfigPath = os.Args[3]
	}
	serverCfg, err := config.LoadServerConfig(serverConfigPath)
	if err != nil {
		fmt.Printf("load server config: %v\n", err)
	}

	// Create API server
	server := api.NewServer(
		config.ServerConfig{
			Host:        serverCfg.Host,
			Port:        serverCfg.Port,
			APIKeyVault: serverCfg.APIKeyVault,
			Version:     "0.1.0",
		},
		handlers.Deps{
			Agents:  registry,
			Version: "0.1.0",
			// Memory: memoryManager,  // wire in when memory package is ready
		},
		logger.Log,
	)

	// Start server in background
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Start() }()

	// Wait for interrupt or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Log.Error("server error", "error", err)
	case sig := <-quit:
		logger.Log.Info("shutdown signal received", "signal", sig)
	}

	// Graceful shutdown — give in-flight requests 10s to finish
	fmt.Println("\nShutting down...")
	logger.Log.Info("shutdown initiated")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		logger.Log.Error("server stop error", "error", err)
	}

	logger.Log.Info("shutdown complete")
}
