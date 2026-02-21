package main

import (
	"context"
	"fmt"
	"os"

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

// getConfigPath returns config path from args or default
func getConfigPath(argIndex int, defaultPath string) string {
	if len(os.Args) > argIndex {
		return os.Args[argIndex]
	}
	return defaultPath
}
