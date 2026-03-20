// cmd/convert.go
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/cmdutil"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	skillcli "github.com/sriramsme/OnlyAgents/pkg/skills/cli"
)

var convertCmd = &cobra.Command{
	Use:   "convert <input-file>",
	Short: "Convert any skill definition to the canonical SKILL.md format",
	Long: `Reads any skill definition file and uses an LLM to rewrite it into the
canonical YAML-frontmatter SKILL.md format, then saves it to the skills
config directory.

Examples:
  onlyagents convert raw_weather.md
  onlyagents convert raw_weather.md -n weather
  onlyagents convert raw_weather.md -n weather -p anthropic
  onlyagents convert raw_weather.md --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runConvert,
}

var (
	convertName     string
	convertProvider string
	convertModel    string
	convertVaultKey string
	convertDryRun   bool
)

func init() {
	rootCmd.AddCommand(convertCmd)
	convertCmd.Flags().StringVarP(&convertName, "name", "n", "", "Output skill name (defaults to LLM-inferred name)")
	convertCmd.Flags().StringVarP(&convertProvider, "provider", "p", "", "LLM provider: anthropic, openai, gemini")
	convertCmd.Flags().StringVarP(&convertModel, "model", "m", "", "LLM model (uses provider default if omitted)")
	convertCmd.Flags().StringVar(&convertVaultKey, "vault-key", "", "Vault key path override, e.g. openai/api_key")
	convertCmd.Flags().BoolVar(&convertDryRun, "dry-run", false, "Print converted skill to stdout instead of saving")
}

// nolint:gocyclo
func runConvert(cmd *cobra.Command, args []string) error {
	logger.Initialize("info", "text")
	ctx := context.Background()

	// ── 1. Read input ─────────────────────────────────────────────────────────
	raw, err := os.ReadFile(args[0]) //nolint:gosec
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	// ── 2. Load vault ─────────────────────────────────────────────────────────
	v, err := vault.Load()
	if err != nil {
		return fmt.Errorf("load vault: %w", err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			fmt.Printf("close vault: %s", err)
		}
	}()

	// ── 3. Resolve provider — interactive if not passed via flags ─────────────
	provider := convertProvider
	model := convertModel

	if provider == "" {
		// Try auto-detect first
		detected, detectedKey, ok := cmdutil.AutoDetectProvider(ctx, v)
		if ok && convertVaultKey == "" {
			convertVaultKey = detectedKey
			provider = detected
			cmdutil.Info("auto-detected provider: %s", provider)
		} else {
			// Fall back to interactive selection
			if err := cmdutil.RunForm(
				huh.NewGroup(
					cmdutil.SelectField("Provider", cmdutil.ProviderOptions(), &provider),
				),
			); err != nil {
				return err
			}
		}
	}

	if model == "" {
		modelOpts, err := cmdutil.ModelOptions(provider)
		if err != nil {
			return err
		}
		if err := cmdutil.RunForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Model").
					OptionsFunc(func() []huh.Option[string] {
						opts, err := cmdutil.ModelOptions(provider)
						if err != nil {
							return []huh.Option[string]{huh.NewOption("(error loading models)", "")}
						}
						return opts
					}, &provider).
					Value(&model),
			),
		); err != nil {
			return err
		}
		_ = modelOpts
	}

	vaultKey := convertVaultKey
	if vaultKey == "" {
		vaultKey = cmdutil.ProviderVaultPath(provider)
	}

	cfg := llm.Config{
		Provider:   provider,
		Model:      model,
		Vault:      v,
		APIKeyPath: vaultKey,
	}
	// ── 4. Build client ───────────────────────────────────────────────────────
	client, err := llm.New(cfg)
	if err != nil {
		return fmt.Errorf("build LLM client: %w", err)
	}

	// ── 5. Convert ────────────────────────────────────────────────────────────
	cmdutil.Info("converting using %s / %s...", provider, model)

	result, err := skillcli.ConvertSKILL(ctx, client, string(raw), skillcli.ConvertOptions{
		SkillName: convertName,
	})
	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	// ── 6. Output ─────────────────────────────────────────────────────────────
	if convertDryRun {
		fmt.Println(result.Content)
		fmt.Printf("\n")
		cmdutil.Success("parsed successfully — %d tool(s) found", len(result.Parsed.Tools))
		return nil
	}

	outName := convertName
	if outName == "" {
		outName = string(result.Parsed.Name)
	}
	outPath := filepath.Join(config.SkillsDir(), outName+".yaml")
	if err := os.WriteFile(outPath, []byte(result.Content), 0o600); err != nil { //nolint:gosec
		return fmt.Errorf("write output: %w", err)
	}

	cmdutil.Success("saved to %s (%d tool(s))", outPath, len(result.Parsed.Tools))
	return nil
}
