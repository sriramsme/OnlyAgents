package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/bootstrap"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	_ "github.com/sriramsme/OnlyAgents/pkg/skills/bootstrap"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run AI agents",
	Long:  `Start the OnlyAgents kernel to run AI agents without API server`,
}

var agentRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the agent kernel",
	RunE:  runAgent,
}

var (
	agentLogLevel            string
	agentLogFormat           string
	agentLogDetailed         bool
	agentLogDetailedLLM      bool
	agentLogDetailedTools    bool
	agentBusBufferSize       int
	agentDefaultID           string
	agentAgentConfigsDir     string
	agentConnectorConfigsDir string
	agentChannelConfigsDir   string
	agentSkillConfigsDir     string
	agentVaultPath           string
)

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentRunCmd)

	// Logging flags
	agentRunCmd.Flags().StringVar(&agentLogLevel, "log-level", "debug", "Log level (debug, info, warn, error)")
	agentRunCmd.Flags().StringVar(&agentLogFormat, "log-format", "json", "Log format (json, text)")
	agentRunCmd.Flags().BoolVar(&agentLogDetailed, "log-detailed", false, "Detailed logging for both LLM and tools")
	agentRunCmd.Flags().BoolVar(&agentLogDetailedLLM, "log-detailed-llm", false, "Detailed LLM calls")
	agentRunCmd.Flags().BoolVar(&agentLogDetailedTools, "log-detailed-tools", false, "Detailed tool calls")
	// Kernel flags
	agentRunCmd.Flags().IntVar(&agentBusBufferSize, "bus-buffer", 100, "Event bus buffer size")
	agentRunCmd.Flags().StringVar(&agentDefaultID, "default-agent", "default", "Default agent ID")
	agentRunCmd.Flags().StringVar(&agentAgentConfigsDir, "agents-dir", "configs/agents/", "Agent configs directory")
	agentRunCmd.Flags().StringVar(&agentConnectorConfigsDir, "connectors-dir", "configs/connectors/", "Connector configs directory")
	agentRunCmd.Flags().StringVar(&agentChannelConfigsDir, "channels-dir", "configs/channels/", "Channel configs directory")
	agentRunCmd.Flags().StringVar(&agentSkillConfigsDir, "skills-dir", "configs/skills/", "Skill configs directory")
	agentRunCmd.Flags().StringVar(&agentVaultPath, "vault", "configs/vault.yaml", "Vault file path")
}

func runAgent(cmd *cobra.Command, args []string) error {
	logger.Initialize(agentLogLevel, agentLogFormat)
	if agentLogDetailed {
		logger.SetTimingDetail(true, true)
	} else {
		logger.SetTimingDetail(agentLogDetailedLLM, agentLogDetailedTools)
	}

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

	fmt.Println("OnlyAgents Kernel v0.1.0")
	fmt.Println("========================")

	// Initialize kernel
	k, err := kernel.NewKernel(kernel.Config{
		BusBufferSize:       agentBusBufferSize,
		DefaultAgentID:      agentDefaultID,
		AgentConfigsDir:     agentAgentConfigsDir,
		ConnectorConfigsDir: agentConnectorConfigsDir,
		ChannelConfigsDir:   agentChannelConfigsDir,
		SkillConfigsDir:     agentSkillConfigsDir,
		VaultPath:           agentVaultPath,
	}, ctx, cancel)

	if err != nil {
		logger.Log.Error("failed to initialize kernel", "error", err)
		return err
	}

	if err := k.Start(); err != nil {
		logger.Log.Error("failed to start kernel", "error", err)
		return err
	}

	logger.Log.Info("kernel started successfully - press Ctrl+C to stop")
	fmt.Println("Press Ctrl+C to stop")

	// Wait for shutdown signal
	<-ctx.Done()

	logger.Log.Info("shutting down kernel")
	if err := k.Stop(); err != nil {
		logger.Log.Error("error shutting down kernel", "error", err)
		return err
	}

	logger.Log.Info("shutdown complete")
	return nil
}
