// cmd/models.go
package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/cmdutil"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Manage and inspect LLM models",
}

var listModelsCmd = &cobra.Command{
	Use:   "list",
	Short: "List supported models (all providers by default)",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := cmd.Flags().GetString("provider")
		if err != nil {
			return err
		}
		return listModels(provider)
	},
}

var infoCmd = &cobra.Command{
	Use:   "info [model]",
	Short: "Get detailed information about a model",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := cmd.Flags().GetString("provider")
		if err != nil {
			return err
		}
		return showModelInfo(args[0], provider)
	},
}

var compareCmd = &cobra.Command{
	Use:   "compare [model1] [model2] [model3...]",
	Short: "Compare multiple models side-by-side",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := cmd.Flags().GetString("provider")
		if err != nil {
			return err
		}
		return compareModels(args, provider)
	},
}

var filterCmd = &cobra.Command{
	Use:   "filter",
	Short: "Filter models by capabilities (interactive)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if any flags were passed — if so, use flag-based mode.
		// Otherwise, launch interactive huh form.
		if cmd.Flags().Changed("tools") ||
			cmd.Flags().Changed("vision") ||
			cmd.Flags().Changed("streaming") ||
			cmd.Flags().Changed("min-tokens") ||
			cmd.Flags().Changed("max-cost") ||
			cmd.Flags().Changed("provider") {
			return filterModelsFromFlags(cmd)
		}
		return filterModelsInteractive()
	},
}

func init() {
	rootCmd.AddCommand(modelsCmd)
	modelsCmd.AddCommand(listModelsCmd)
	modelsCmd.AddCommand(infoCmd)
	modelsCmd.AddCommand(compareCmd)
	modelsCmd.AddCommand(filterCmd)

	for _, c := range []*cobra.Command{listModelsCmd, infoCmd, compareCmd, filterCmd} {
		c.Flags().StringP("provider", "p", "all", "LLM provider (all, openai, anthropic, gemini)")
	}

	filterCmd.Flags().Int("min-tokens", 0, "Minimum max output tokens")
	filterCmd.Flags().Float64("max-cost", 0, "Maximum output cost per 1M tokens")
	filterCmd.Flags().Bool("streaming", false, "Require streaming support")
	filterCmd.Flags().Bool("tools", false, "Require tool calling support")
	filterCmd.Flags().Bool("vision", false, "Require vision support")
}

// ── list ──────────────────────────────────────────────────────────────────────

func listModels(provider string) error {
	registry, err := cmdutil.LLMRegistry(provider)
	if err != nil {
		return err
	}

	models := llm.GetAllModelsInfo(registry)
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider == models[j].Provider {
			return models[i].Name < models[j].Name
		}
		return models[i].Provider < models[j].Provider
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, cmdutil.StyleHeader.Render("MODEL\tPROVIDER\tCONTEXT\tMAX OUT\tTOOLS\tVISION\tCOST\tDESCRIPTION"))
	fmt.Fprintln(w, "─────\t────────\t───────\t───────\t─────\t──────\t────\t───────────")

	for _, info := range models {
		c := info.Capabilities
		fmt.Fprintf(w, "%s\t%s\t%dk\t%dk\t%s\t%s\t$%.2f→$%.2f\t%s\n",
			info.Name,
			info.Provider,
			c.ContextWindow/1000,
			c.MaxTokens/1000,
			cmdutil.YesNo(c.SupportsToolCalling),
			cmdutil.YesNo(c.SupportsVision),
			c.InputCostPer1M,
			c.OutputCostPer1M,
			cmdutil.Truncate(c.Description, 40),
		)
		if c.Deprecated {
			fmt.Fprintf(w, "\t\t\t\t\t\t\t%s\n",
				cmdutil.StyleYellow.Render("⚠ DEPRECATED — use "+c.ReplacedBy),
			)
		}
	}

	return w.Flush()
}

// ── info ──────────────────────────────────────────────────────────────────────

func showModelInfo(model, provider string) error {
	registry, err := cmdutil.LLMRegistry(provider)
	if err != nil {
		return err
	}

	info, err := llm.GetModelInfo(model, registry)
	if err != nil {
		return err
	}

	c := info.Capabilities

	fmt.Println(cmdutil.StyleBorder.Render(
		cmdutil.StyleHeader.Render(info.Name) + " · " + info.Provider,
	))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Description\t%s\n", c.Description)
	fmt.Fprintf(w, "Context window\t%d tokens\n", c.ContextWindow)
	fmt.Fprintf(w, "Max output\t%d tokens\n", c.MaxTokens)
	fmt.Fprintf(w, "Default output\t%d tokens\n", c.DefaultMaxTokens)

	if c.SupportsTemperature {
		fmt.Fprintf(w, "Temperature\t%.1f – %.1f (default %.1f)\n",
			c.MinTemperature, c.MaxTemperature, c.DefaultTemperature)
	} else {
		fmt.Fprintf(w, "Temperature\t%s\n", cmdutil.StyleDim.Render("not supported"))
	}

	fmt.Fprintf(w, "Tool calling\t%s\n", cmdutil.YesNo(c.SupportsToolCalling))
	fmt.Fprintf(w, "Vision\t%s\n", cmdutil.YesNo(c.SupportsVision))
	fmt.Fprintf(w, "Streaming\t%s\n", cmdutil.YesNo(c.SupportsStreaming))

	if c.SupportsPromptCaching {
		fmt.Fprintf(w, "Prompt caching\t%s\n", cmdutil.YesNo(true))
	}
	if c.IsReasoningModel {
		fmt.Fprintf(w, "Reasoning model\t%s\n", cmdutil.YesNo(true))
	}

	fmt.Fprintf(w, "Input cost\t$%.4f / 1M tokens\n", c.InputCostPer1M)
	fmt.Fprintf(w, "Output cost\t$%.4f / 1M tokens\n", c.OutputCostPer1M)

	if c.Deprecated {
		fmt.Fprintf(w, "Status\t%s\n",
			cmdutil.StyleYellow.Render("⚠ DEPRECATED — use "+c.ReplacedBy),
		)
	}

	return w.Flush()
}

// ── compare ───────────────────────────────────────────────────────────────────

func compareModels(models []string, provider string) error {
	registry, err := cmdutil.LLMRegistry(provider)
	if err != nil {
		return err
	}
	comparison, err := llm.CompareModels(models, registry)
	if err != nil {
		return err
	}
	fmt.Println(comparison)
	return nil
}

// ── filter (interactive) ──────────────────────────────────────────────────────

func filterModelsInteractive() error {
	var (
		provider         = "all"
		requireTools     bool
		requireVision    bool
		requireStreaming bool
	)

	if err := cmdutil.RunForm(
		huh.NewGroup(
			cmdutil.SelectField("Provider", cmdutil.ProviderOptions(), &provider),
		),
		huh.NewGroup(
			huh.NewConfirm().Title("Requires tool calling?").Value(&requireTools),
			huh.NewConfirm().Title("Requires vision?").Value(&requireVision),
			huh.NewConfirm().Title("Requires streaming?").Value(&requireStreaming),
		),
	); err != nil {
		return err
	}

	return applyFilter(provider, llm.CapabilityFilter{
		RequireTools:     requireTools,
		RequireVision:    requireVision,
		RequireStreaming: requireStreaming,
	})
}

// ── filter (flag-based) ───────────────────────────────────────────────────────

func filterModelsFromFlags(cmd *cobra.Command) error {
	provider, err := cmd.Flags().GetString("provider")
	if err != nil {
		return err
	}

	filter := llm.CapabilityFilter{}

	if v, err := cmd.Flags().GetInt("min-tokens"); err != nil {
		return err
	} else if v > 0 {
		filter.MinMaxTokens = &v
	}

	if v, err := cmd.Flags().GetFloat64("max-cost"); err != nil {
		return err
	} else if v > 0 {
		filter.MaxCostPer1M = &v
	}

	if filter.RequireStreaming, err = cmd.Flags().GetBool("streaming"); err != nil {
		return err
	}

	if filter.RequireTools, err = cmd.Flags().GetBool("tools"); err != nil {
		return err
	}

	if filter.RequireVision, err = cmd.Flags().GetBool("vision"); err != nil {
		return err
	}

	return applyFilter(provider, filter)
}

// ── shared output ─────────────────────────────────────────────────────────────

func applyFilter(provider string, filter llm.CapabilityFilter) error {
	registry, err := cmdutil.LLMRegistry(provider)
	if err != nil {
		return err
	}

	models := llm.FilterModels(filter, registry)
	if len(models) == 0 {
		fmt.Println(cmdutil.StyleDim.Render("No models match the filter criteria."))
		return nil
	}

	fmt.Printf("%s\n\n", cmdutil.StyleHeader.Render(fmt.Sprintf("%d model(s) found", len(models))))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, info := range models {
		c := info.Capabilities
		fmt.Fprintf(w, "%s\t%s\t%dk ctx\t$%.2f→$%.2f/1M\t%s\n",
			cmdutil.StyleBold.Render(info.Name),
			info.Provider,
			c.ContextWindow/1000,
			c.InputCostPer1M,
			c.OutputCostPer1M,
			cmdutil.Truncate(c.Description, 50),
		)
	}
	return w.Flush()
}
