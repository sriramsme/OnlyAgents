package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/cmdutil"
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/bootstrap"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	_ "github.com/sriramsme/OnlyAgents/pkg/skills/bootstrap"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage and run AI agents",
}

// ── run ───────────────────────────────────────────────────────────────────────

var agentRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the agent kernel (no API server)",
	RunE:  runAgent,
}

var (
	agentLogLevel         string
	agentLogFormat        string
	agentLogDetailed      bool
	agentLogDetailedLLM   bool
	agentLogDetailedTools bool
	agentBusBufferSize    int
)

func init() {
	rootCmd.AddCommand(agentCmd)

	// run
	agentCmd.AddCommand(agentRunCmd)
	agentRunCmd.Flags().StringVar(&agentLogLevel, "log-level", "debug", "Log level (debug, info, warn, error)")
	agentRunCmd.Flags().StringVar(&agentLogFormat, "log-format", "json", "Log format (json, text)")
	agentRunCmd.Flags().BoolVar(&agentLogDetailed, "log-detailed", false, "Detailed logging for both LLM and tools")
	agentRunCmd.Flags().BoolVar(&agentLogDetailedLLM, "log-detailed-llm", false, "Detailed LLM calls")
	agentRunCmd.Flags().BoolVar(&agentLogDetailedTools, "log-detailed-tools", false, "Detailed tool calls")
	agentRunCmd.Flags().IntVar(&agentBusBufferSize, "bus-buffer", 100, "Event bus buffer size")

	// management
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentEnableCmd)
	agentCmd.AddCommand(agentDisableCmd)
	agentCmd.AddCommand(agentViewCmd)
	agentCmd.AddCommand(agentEditCmd)

	agentViewCmd.Flags().String("field", "", "Print a specific field value")
	agentViewCmd.Flags().Bool("raw", false, "Dump raw YAML")
}

func runAgent(cmd *cobra.Command, args []string) error {
	logger.Initialize(agentLogLevel, agentLogFormat)
	if agentLogDetailed {
		logger.SetTimingDetail(true, true)
	} else {
		logger.SetTimingDetail(agentLogDetailedLLM, agentLogDetailedTools)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigChan
		logger.Log.Info("received shutdown signal", "signal", sig.String())
		cancel()
	}()

	fmt.Println("OnlyAgents Kernel v0.1.0")
	fmt.Println("========================")

	k, err := kernel.NewKernel(ctx, cancel, nil)
	if err != nil {
		logger.Log.Error("failed to initialize kernel", "error", err)
		return err
	}
	if err := k.Start(); err != nil {
		logger.Log.Error("failed to start kernel", "error", err)
		return err
	}

	logger.Log.Info("kernel started — press Ctrl+C to stop")
	fmt.Println("Press Ctrl+C to stop")

	<-ctx.Done()
	logger.Log.Info("shutting down kernel")
	if err := k.Stop(); err != nil {
		logger.Log.Error("error shutting down kernel", "error", err)
		return err
	}
	logger.Log.Info("shutdown complete")
	return nil
}

// ── list ──────────────────────────────────────────────────────────────────────

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		agents, err := cmdutil.AgentRegistry(paths.Agents)
		if err != nil {
			return err
		}
		if len(agents) == 0 {
			fmt.Println(cmdutil.StyleDim.Render("No agents configured."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, cmdutil.StyleHeader.Render("ID\tNAME\tROLE\tSTATUS\tPROVIDER\tMODEL"))
		fmt.Fprintln(w, "──\t────\t────\t──────\t────────\t─────")
		for _, a := range agents {
			role := "sub-agent"
			if a.IsExecutive {
				role = "executive"
			} else if a.IsGeneral {
				role = "general"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				a.ID,
				a.Name,
				role,
				cmdutil.EnabledLabel(a.Enabled),
				a.LLM.Provider,
				a.LLM.Model,
			)
		}
		return w.Flush()
	},
}

// ── enable / disable ──────────────────────────────────────────────────────────

var agentEnableCmd = &cobra.Command{
	Use:   "enable <id>",
	Short: "Enable an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		if err := cmdutil.AgentSetEnabled(paths.Agents, args[0], true); err != nil {
			return err
		}
		cmdutil.Success("%s enabled", args[0])
		return nil
	},
}

var agentDisableCmd = &cobra.Command{
	Use:   "disable <id>",
	Short: "Disable an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		if err := cmdutil.AgentSetEnabled(paths.Agents, args[0], false); err != nil {
			return err
		}
		cmdutil.Warn("%s disabled", args[0])
		return nil
	},
}

// ── view ──────────────────────────────────────────────────────────────────────

var agentViewCmd = &cobra.Command{
	Use:   "view <id>",
	Short: "View an agent config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		agents, err := cmdutil.AgentRegistry(paths.Agents)
		if err != nil {
			return err
		}
		agent, err := cmdutil.FindAgent(agents, args[0])
		if err != nil {
			return err
		}
		raw, err := cmd.Flags().GetBool("raw")
		if err != nil {
			return err
		}
		field, err := cmd.Flags().GetString("field")
		if err != nil {
			return err
		}
		return cmdutil.ViewResource(cmdutil.AgentConfigPath(paths.Agents, args[0]), agent, field, raw)
	},
}

// ── edit ──────────────────────────────────────────────────────────────────────

var agentEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit an agent config interactively",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		agents, err := cmdutil.AgentRegistry(paths.Agents)
		if err != nil {
			return err
		}
		agent, err := cmdutil.FindAgent(agents, args[0])
		if err != nil {
			return err
		}

		// Pre-populate from existing config
		name := agent.Name
		provider := agent.LLM.Provider
		enabled := agent.Enabled

		// Build model options based on current provider
		modelOpts, err := cmdutil.ModelOptions(provider)
		if err != nil {
			return err
		}
		model := agent.LLM.Model

		if err := cmdutil.RunForm(
			huh.NewGroup(
				cmdutil.InputField("Name", agent.Name, &name),
				cmdutil.ConfirmField("Enabled", &enabled),
			),
			huh.NewGroup(
				cmdutil.SelectField("Provider", cmdutil.ProviderOptions(), &provider),
			),
			huh.NewGroup(
				cmdutil.SelectField("Model", modelOpts, &model),
			),
		); err != nil {
			return err
		}

		if err := cmdutil.AgentSetLLM(paths.Agents, args[0], provider, model, cmdutil.ProviderVaultPath(provider)); err != nil {
			return err
		}
		if err := cmdutil.AgentSetEnabled(paths.Agents, args[0], enabled); err != nil {
			return err
		}

		// Update name separately since AgentSetLLM only touches llm block
		configPath := cmdutil.AgentConfigPath(paths.Agents, args[0])
		var raw map[string]any
		if err := cmdutil.ReadYAML(configPath, &raw); err != nil {
			return err
		}
		raw["name"] = name
		if err := cmdutil.WriteYAML(configPath, raw); err != nil {
			return err
		}

		cmdutil.Success("agent %s updated", args[0])
		return nil
	},
}
