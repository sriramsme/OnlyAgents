package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/config"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

func loadConfig(path string) (*config.Config, vault.Vault, error) {
	cfg, v, err := config.Load(path)
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("validate config: %w", err)
	}

	// Initialize logging after config is validated
	logger.Initialize(cfg.Logging.Level, cfg.Logging.Format)
	logger.Log.Info("configuration loaded",
		"agent_id", cfg.Agent.ID,
		"agent_name", cfg.Agent.Name,
		"provider", cfg.LLM.Provider,
		"model", cfg.LLM.Model)

	return cfg, v, nil
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

func main() {
	fmt.Println("OnlyAgents v0.1.0")
	fmt.Println("==================")
	fmt.Println()

	// Load .env file first (if it exists)
	if err := vault.LoadDotEnv(".env"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Error loading .env file: %v\n", err)
	}

	// Load config
	configPath := "agent.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, v, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	defer func() {
		if err := v.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing vault: %v\n", err)
		}
	}()

	// Create LLM client using factory
	llmClient, err := createLLMClient(cfg, v)
	if err != nil {
		logger.Log.Error("failed to create LLM client", "error", err)
		os.Exit(1)
	}

	// Create agent config
	agentConfig := kernel.Config{
		ID:             cfg.Agent.ID,
		MaxConcurrency: cfg.Agent.MaxConcurrency,
		BufferSize:     cfg.Agent.BufferSize,
		LLMClient:      llmClient,
	}

	// Create agent
	agent, err := kernel.NewAgent(agentConfig)
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
		"agent_id", cfg.Agent.ID,
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
