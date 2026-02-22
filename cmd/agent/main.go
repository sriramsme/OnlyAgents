package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/sriramsme/OnlyAgents/pkg/channels/bootstrap"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
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
	}, ctx, cancel)

	if err != nil {
		logger.Log.Error("failed to initialize kernel", "error", err)
		os.Exit(1)
	}

	if err := k.Start(); err != nil {
		logger.Log.Error("failed to start kernel", "error", err)
		os.Exit(1)
	}

	logger.Log.Info("server started successfully - press Ctrl+C to stop")

	// Wait for shutdown signal
	<-ctx.Done()

	logger.Log.Info("shutting down kernel")
	if err := k.Stop(); err != nil {
		logger.Log.Error("error shutting down kernel", "error", err)
		os.Exit(1)
	}

	logger.Log.Info("shutdown complete")
}
