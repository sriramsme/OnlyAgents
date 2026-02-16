package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sriramsme/OnlyAgents/pkg/config"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

func loadConfig(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	// Initialize logging after config is validated
	logger.Initialize(cfg.Logging.Level, cfg.Logging.Format)

	logger.Log.Info("Configuration loaded",
		"agent_id", cfg.Agent.ID,
		"agent_name", cfg.Agent.Name,
	)

	return cfg, nil
}

// create llm client
func createLLMClient(cfg *config.Config) (llm.Client, error) {
	if cfg.LLM.Provider == "" {
		return nil, fmt.Errorf("llm provider must be configured")
	}

	llmClient, err := llm.NewClient(llm.Config{
		Provider:    llm.Provider(cfg.LLM.Provider),
		Model:       cfg.LLM.Model,
		APIKey:      cfg.LLM.APIKey,
		BaseURL:     cfg.LLM.BaseURL,
		MaxTokens:   4096,
		Temperature: 1.0,
	})
	if err != nil {
		return nil, err
	}

	logger.Log.Info("LLM client initialized",
		"provider", llmClient.Provider(),
		"model", llmClient.Model())

	return llmClient, nil
}

func main() {
	fmt.Println("OpenAgent v0.1.0")
	fmt.Println("================")

	// Load config
	cfg, err := loadConfig("agent.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Create LLM client
	llmClient, err := createLLMClient(cfg)
	if err != nil {
		logger.Log.Error("Failed to create LLM client", "err", err)
		os.Exit(1)
	}
	// Create agent config
	agentConfig := kernel.Config{
		ID:             "user.assistant.main",
		MaxConcurrency: 10,
		BufferSize:     100,
		LLMClient:      llmClient,
	}

	// Create agent
	agent, err := kernel.NewAgent(agentConfig)
	if err != nil {
		logger.Log.Error("Failed to create agent", "err", err)
		os.Exit(1)
	}

	// Start agent
	if err := agent.Start(); err != nil {
		logger.Log.Error("Failed to start agent", "err", err)
		os.Exit(1)
	}

	// Wait for SIGINT or SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	fmt.Println("Agent started. Press Ctrl+C to stop the agent")
	<-ctx.Done()

	// Graceful shutdown
	fmt.Println("\nShutting down...")
	if err := agent.Stop(); err != nil {
		logger.Log.Error("Error during shutdown", "err", err)
	}
}
