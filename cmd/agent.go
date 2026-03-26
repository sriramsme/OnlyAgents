package cmd

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/cmdutil"
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/bootstrap"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/bootstrap"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	_ "github.com/sriramsme/OnlyAgents/pkg/skills/bootstrap"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agents",
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

	agentCmd.Flags().StringVar(&agentLogLevel, "log-level", "debug", "Log level (debug, info, warn, error)")
	agentCmd.Flags().StringVar(&agentLogFormat, "log-format", "json", "Log format (json, text)")
	agentCmd.Flags().BoolVar(&agentLogDetailed, "log-detailed", false, "Detailed logging for both LLM and tools")
	agentCmd.Flags().BoolVar(&agentLogDetailedLLM, "log-detailed-llm", false, "Detailed LLM calls")
	agentCmd.Flags().BoolVar(&agentLogDetailedTools, "log-detailed-tools", false, "Detailed tool calls")
	agentCmd.Flags().IntVar(&agentBusBufferSize, "bus-buffer", 100, "Event bus buffer size")

	// management
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentEnableCmd)
	agentCmd.AddCommand(agentDisableCmd)
	agentCmd.AddCommand(agentViewCmd)
	agentCmd.AddCommand(agentEditCmd)

	agentViewCmd.Flags().String("field", "", "Print a specific field value")
	agentViewCmd.Flags().Bool("raw", false, "Dump raw YAML")
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
		rows := make([][]string, len(agents))
		dimmed := make([]bool, len(agents))
		for i, a := range agents {
			role := "sub-agent"
			if a.IsExecutive {
				role = "executive"
			} else if a.IsGeneral {
				role = "general"
			}
			rows[i] = []string{a.ID, a.Name, role, cmdutil.EnabledLabel(a.Enabled), a.LLM.Provider, a.LLM.Model}
			dimmed[i] = !a.Enabled
		}
		cmdutil.PrintTable([]string{"ID", "NAME", "ROLE", "STATUS", "PROVIDER", "MODEL"}, rows, dimmed)
		return nil
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
		model := agent.LLM.Model

		if err := cmdutil.RunForm(
			huh.NewGroup(
				cmdutil.InputField("Name", agent.Name, &name),
				cmdutil.ConfirmField("Enabled", &enabled),
			),
			huh.NewGroup(
				cmdutil.SelectField("Provider", cmdutil.ProviderOptions(), &provider),
				huh.NewSelect[string]().
					Title("Model").
					OptionsFunc(func() []huh.Option[string] {
						opts, err := cmdutil.ModelOptions(provider)
						if err != nil {
							return []huh.Option[string]{huh.NewOption("(no models found)", "")}
						}
						return opts
					}, &provider).
					Value(&model),
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
