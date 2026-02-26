// cmd/models.go
package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/anthropic"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/gemini"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/openai"
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
		model := args[0]
		provider, err := cmd.Flags().GetString("provider")
		if err != nil {
			return err
		}
		return showModelInfo(model, provider)
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
	Short: "Filter models by capabilities",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := cmd.Flags().GetString("provider")
		if err != nil {
			return err
		}

		filter := llm.CapabilityFilter{}

		minTokens, err := cmd.Flags().GetInt("min-tokens")
		if err != nil {
			return err
		}
		if minTokens > 0 {
			filter.MinMaxTokens = &minTokens
		}

		maxCost, err := cmd.Flags().GetFloat64("max-cost")
		if err != nil {
			return err
		}
		if maxCost > 0 {
			filter.MaxCostPer1M = &maxCost
		}

		requireStreaming, err := cmd.Flags().GetBool("streaming")
		if err != nil {
			return err
		}
		filter.RequireStreaming = requireStreaming

		requireTools, err := cmd.Flags().GetBool("tools")
		if err != nil {
			return err
		}
		filter.RequireTools = requireTools

		requireVision, err := cmd.Flags().GetBool("vision")
		if err != nil {
			return err
		}
		filter.RequireVision = requireVision

		return filterModels(filter, provider)
	},
}

func init() {
	rootCmd.AddCommand(modelsCmd)
	modelsCmd.AddCommand(listModelsCmd)
	modelsCmd.AddCommand(infoCmd)
	modelsCmd.AddCommand(compareCmd)
	modelsCmd.AddCommand(filterCmd)

	// Add provider flag to all subcommands
	for _, cmd := range []*cobra.Command{listModelsCmd, infoCmd, compareCmd, filterCmd} {
		cmd.Flags().StringP("provider", "p", "all",
			"LLM provider (all, openai, anthropic, gemini)")
	}

	filterCmd.Flags().Int("min-tokens", 0, "Minimum max tokens")
	filterCmd.Flags().Float64("max-cost", 0, "Maximum output cost per 1M tokens")
	filterCmd.Flags().Bool("streaming", false, "Require streaming support")
	filterCmd.Flags().Bool("tools", false, "Require tool calling support")
	filterCmd.Flags().Bool("vision", false, "Require vision support")
}

// getRegistry returns the model registry for the specified provider
func getRegistry(provider string) (map[string]llm.ModelCapabilities, error) {
	switch provider {
	case "all":
		return getAllRegistries(), nil
	case "openai":
		return openai.ModelRegistry, nil
	case "anthropic":
		return anthropic.ModelRegistry, nil
	case "gemini":
		return gemini.ModelRegistry, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}
func getAllRegistries() map[string]llm.ModelCapabilities {
	all := map[string]llm.ModelCapabilities{}

	for k, v := range openai.ModelRegistry {
		all[k] = v
	}
	for k, v := range anthropic.ModelRegistry {
		all[k] = v
	}
	for k, v := range gemini.ModelRegistry {
		all[k] = v
	}

	return all
}
func listModels(provider string) error {
	registry, err := getRegistry(provider)
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
	if _, err := fmt.Fprintln(w, "MODEL\tPROVIDER\tCONTEXT\tMAX OUT\tTOOLS\tVISION\tCOST\tDESCRIPTION"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "-----\t--------\t-------\t-------\t-----\t------\t-----------\t-----------"); err != nil {
		return err
	}

	for _, info := range models {
		c := info.Capabilities

		if _, err := fmt.Fprintf(w, "%s\t%s\t%dk\t%dk\t%s\t%s\t$%.2f→$%.2f\t%s\n",
			info.Name,
			info.Provider,
			c.ContextWindow/1000,
			c.MaxTokens/1000,
			yesNo(c.SupportsToolCalling),
			yesNo(c.SupportsVision),
			c.InputCostPer1M,
			c.OutputCostPer1M,
			truncate(c.Description, 40),
		); err != nil {
			return err
		}

		if c.Deprecated {
			if _, err := fmt.Fprintf(w, "\t\t\t\t\t\t\t⚠️  DEPRECATED - Use %s instead\n", c.ReplacedBy); err != nil {
				return err
			}
		}
	}

	return w.Flush()
}

func showModelInfo(model, provider string) error {
	registry, err := getRegistry(provider)
	if err != nil {
		return err
	}

	info, err := llm.GetModelInfo(model, registry)
	if err != nil {
		return err
	}

	c := info.Capabilities

	fmt.Printf("Model: %s\n", info.Name)
	fmt.Printf("Provider: %s\n", info.Provider)
	fmt.Printf("Description: %s\n\n", c.Description)

	fmt.Printf("Capabilities:\n")
	fmt.Printf("  Context Window:    %d tokens\n", c.ContextWindow)
	fmt.Printf("  Max Output:        %d tokens\n", c.MaxTokens)
	fmt.Printf("  Default Output:    %d tokens\n", c.DefaultMaxTokens)

	if c.SupportsTemperature {
		fmt.Printf("  Temperature:       %.1f - %.1f (default: %.1f)\n",
			c.MinTemperature, c.MaxTemperature, c.DefaultTemperature)
	} else {
		fmt.Printf("  Temperature:       Not supported\n")
	}

	fmt.Printf("\nFeatures:\n")
	for _, feature := range info.Features {
		fmt.Printf("  ✓ %s\n", feature)
	}

	fmt.Printf("\nPricing:\n")
	fmt.Printf("  Input:  $%.2f per 1M tokens\n", c.InputCostPer1M)
	fmt.Printf("  Output: $%.2f per 1M tokens\n", c.OutputCostPer1M)

	// Provider-specific features (now directly on capabilities)
	if c.SupportsPromptCaching || c.SupportsAudio || c.IsReasoningModel {
		fmt.Printf("\nAdvanced Features:\n")
		if c.SupportsPromptCaching {
			fmt.Printf("  ✓ Prompt Caching\n")
		}
		if c.SupportsAudio {
			fmt.Printf("  ✓ Audio\n")
		}
		if c.IsReasoningModel {
			fmt.Printf("  ✓ Advanced Reasoning\n")
		}
	}

	if c.Deprecated {
		fmt.Printf("\n⚠️  DEPRECATED - Replaced by: %s\n", c.ReplacedBy)
	}

	return nil
}

func compareModels(models []string, provider string) error {
	registry, err := getRegistry(provider)
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

func filterModels(filter llm.CapabilityFilter, provider string) error {
	registry, err := getRegistry(provider)
	if err != nil {
		return err
	}

	models := llm.FilterModels(filter, registry)

	if len(models) == 0 {
		fmt.Println("No models match the filter criteria")
		return nil
	}

	fmt.Printf("Found %d matching models:\n\n", len(models))

	for _, info := range models {
		fmt.Printf("• %s - %s\n", info.Name, info.Capabilities.Description)
		fmt.Printf("  Context: %dk | Max: %dk | Cost: $%.2f→$%.2f/1M\n",
			info.Capabilities.ContextWindow/1000,
			info.Capabilities.MaxTokens/1000,
			info.Capabilities.InputCostPer1M,
			info.Capabilities.OutputCostPer1M)
	}

	return nil
}

func yesNo(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
