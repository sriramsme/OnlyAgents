package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/config"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

func main() {
	fmt.Println("OnlyAgents v0.1.0")
	fmt.Println("==================")
	fmt.Println()

	vaultPath := "configs/vault.yaml"
	if len(os.Args) > 1 {
		vaultPath = os.Args[1]
	}
	vault, err := config.LoadVault(vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load vault config: %v\n", err)
		os.Exit(1)
	}

	defer func() {
		if err := vault.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing vault: %v\n", err)
		}
	}()
	// Load config
	configPath := "configs/agents/messenger.yaml"
	if len(os.Args) > 2 {
		configPath = os.Args[2]
	}

	cfg, err := loadConfig(configPath, vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Create LLM client using factory
	llmClient, err := createLLMClient(cfg, cfg.GetVault())
	if err != nil {
		logger.Log.Error("failed to create LLM client", "error", err)
		os.Exit(1)
	}

	// Create agent
	agent, err := kernel.NewAgent(*cfg, llmClient)
	if err != nil {
		logger.Log.Error("failed to create agent", "error", err)
		os.Exit(1)
	}

	// Register skills (TODO: Load from config)
	// Example:
	// agent.RegisterSkill(skills.NewCalendarSkill())
	// agent.RegisterSkill(skills.NewEmailSkill())

	// Start agent
	if err := agent.Start(); err != nil {
		logger.Log.Error("failed to start agent", "error", err)
		os.Exit(1)
	}

	// Example: Execute a simple task (for testing)
	if true { // Set to true to test
		ctx := context.Background()
		response, err := agent.Execute(ctx, "What's 2+2?")
		if err != nil {
			logger.Log.Error("execution failed", "error", err)
		} else {
			fmt.Println("\n=== Test Execution ===")
			fmt.Println("Response:", response)
			fmt.Println("======================")
		}
	}

	// Wait for interrupt signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Log.Info("agent running",
		"agent_id", cfg.ID,
		"press", "Ctrl+C to stop")
	fmt.Println("Agent running. Press Ctrl+C to stop...")

	<-ctx.Done()

	// Graceful shutdown
	fmt.Println("\nShutting down...")
	logger.Log.Info("shutdown initiated")

	if err := agent.Stop(); err != nil {
		logger.Log.Error("error during shutdown", "error", err)
		os.Exit(1)
	}

	logger.Log.Info("shutdown complete")
}

func loadConfig(path string, vault vault.Vault) (*config.Config, error) {
	cfg, err := config.LoadAgentConfig(path, vault)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	// Initialize logging after config is validated
	logger.Initialize(cfg.Logging.Level, cfg.Logging.Format)
	logger.Log.Info("configuration loaded",
		"agent_id", cfg.ID,
		"agent_name", cfg.Name,
		"provider", cfg.LLM.Provider,
		"model", cfg.LLM.Model)

	return cfg, nil
}

// createLLMClient creates an LLM client using the factory pattern
func createLLMClient(cfg *config.Config, vault vault.Vault) (llm.Client, error) {
	if cfg.LLM.Provider == "" {
		return nil, fmt.Errorf("llm provider must be configured")
	}

	// Use factory to create client from config
	factory := llm.NewFactory(cfg, vault)
	client, err := factory.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	logger.Log.Info("llm client initialized",
		"provider", client.Provider(),
		"model", client.Model())

	return client, nil
}
