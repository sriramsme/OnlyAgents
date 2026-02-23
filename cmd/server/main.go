package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/api"
	"github.com/sriramsme/OnlyAgents/internal/api/handlers"
	"github.com/sriramsme/OnlyAgents/internal/config"
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/bootstrap"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	_ "github.com/sriramsme/OnlyAgents/pkg/skills/bootstrap"
)

func main() {
	logger.Initialize("debug", "json")

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Handle signals in goroutine
	go func() {
		sig := <-sigChan
		logger.Log.Info("received shutdown signal", "signal", sig.String())
		cancel()
	}()

	fmt.Println("OnlyAgents Server v0.1.0")
	fmt.Println("=========================")

	k, err := kernel.NewKernel(kernel.Config{
		BusBufferSize:       100,
		DefaultAgentID:      "default",
		AgentConfigsDir:     "configs/agents/",
		ConnectorConfigsDir: "configs/connectors/",
		ChannelConfigsDir:   "configs/channels/",
		SkillConfigsDir:     "configs/skills/",
		VaultPath:           "configs/vault.yaml",
	}, ctx, cancel)

	if err != nil {
		logger.Log.Error("failed to initialize kernel", "error", err)
		os.Exit(1)
	}

	if err := k.Start(); err != nil {
		logger.Log.Error("failed to start kernel", "error", err)
		os.Exit(1)
	}

	fmt.Println("server started successfully - press Ctrl+C to stop")

	// Start API server
	serverConfig := mustLoadServerConfig()
	server := createServer(serverConfig, k)

	u := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(serverConfig.Host, strconv.Itoa(serverConfig.Port)),
	}
	fmt.Println("server started successfully", "url", u.String())

	runServer(server)
	if err := k.Start(); err != nil {
		logger.Log.Error("failed to start kernel", "error", err)
		os.Exit(1) // os.Exit lives HERE, not in the library
	}

	<-ctx.Done()

	logger.Log.Info("shutting down kernel")
	if err := k.Stop(); err != nil {
		logger.Log.Error("error shutting down kernel", "error", err)
		os.Exit(1)
	}

	logger.Log.Info("shutdown complete")
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
