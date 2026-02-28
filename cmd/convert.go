// cmd/convert.go
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/anthropic"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/gemini"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/openai"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	skillcli "github.com/sriramsme/OnlyAgents/pkg/skills/cli"
)

var convertCmd = &cobra.Command{
	Use:   "convert <input-file>",
	Short: "Convert an unformatted SKILL.md to the canonical format",
	Long: `Reads any skill definition file and uses an LLM to rewrite it into the
canonical YAML-frontmatter SKILL.md format, then saves it to the skills
config directory.

Examples:
  onlyagents convert raw_weather.md -n weather
  onlyagents convert raw_weather.md -n weather -p anthropic -m claude-3-5-sonnet-20241022
  onlyagents convert raw_weather.md -n weather -p openai
  onlyagents convert raw_weather.md -n weather --vault-key openai/api_key`,

	Args: cobra.ExactArgs(1),
	RunE: runConvert,
}

var (
	convertName      string // -n / --name
	convertProvider  string // -p / --provider
	convertModel     string // -m / --model
	convertOutputDir string // --output-dir
	convertVaultPath string // --vault
	convertVaultKey  string // --vault-key
	convertDryRun    bool   // --dry-run
)

// providerDefaults maps provider names to their default model and vault key path.
// Order matters for auto-detection: first provider with a vault key wins.
var providerDefaults = []struct {
	name     string
	model    string
	vaultKey string
}{
	{"anthropic", "claude-3-5-sonnet-20241022", "anthropic/api_key"},
	{"openai", "gpt-4o", "openai/api_key"},
	{"gemini", "gemini-1.5-pro", "gemini/api_key"},
}

func init() {
	rootCmd.AddCommand(convertCmd)

	convertCmd.Flags().StringVarP(&convertName, "name", "n", "", "Name to save as in the output directory (defaults to input filename stem)")
	convertCmd.Flags().StringVarP(&convertProvider, "provider", "p", "", "LLM provider: anthropic, openai, gemini (auto-detected from vault if omitted)")
	convertCmd.Flags().StringVarP(&convertModel, "model", "m", "", "LLM model (uses provider default if omitted)")
	convertCmd.Flags().StringVar(&convertOutputDir, "output-dir", "configs/skills/", "Directory to write the converted skill to")
	convertCmd.Flags().StringVar(&convertVaultPath, "vault", "configs/vault.yaml", "Path to the vault file containing API keys")
	convertCmd.Flags().StringVar(&convertVaultKey, "vault-key", "", "Vault key path for the API key, e.g. openai/api_key")
	convertCmd.Flags().BoolVar(&convertDryRun, "dry-run", false, "Print the converted skill to stdout instead of writing it to disk")
}

func runConvert(cmd *cobra.Command, args []string) error {
	logger.Initialize("info", "text")

	ctx := context.Background()

	inputPath := args[0]

	// ── 1. Read the raw input file ────────────────────────────────────────────
	raw, err := os.ReadFile(inputPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("read input file: %w", err)
	}

	// ── 3. Load vault ─────────────────────────────────────────────────────────
	v, err := config.LoadVault(convertVaultPath)
	if err != nil {
		return fmt.Errorf("load vault: %w", err)
	}

	// ── 4. Resolve LLM client ─────────────────────────────────────────────────
	client, resolvedProvider, err := resolveClient(ctx, v)
	if err != nil {
		return fmt.Errorf("resolve LLM client: %w", err)
	}

	// ── 5. Convert ────────────────────────────────────────────────────────────
	fmt.Printf("Converting %q using %s...\n", inputPath, resolvedProvider)

	result, err := skillcli.ConvertSKILL(
		context.Background(),
		client,
		string(raw),
		skillcli.ConvertOptions{SkillName: convertName},
	)
	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	// ── 6. Output ─────────────────────────────────────────────────────────────
	if convertDryRun {
		fmt.Println(result.Content)
		fmt.Printf("\n✓ Parsed successfully: %d tool(s) found\n", len(result.Parsed.Commands))
		return nil
	}

	if err := os.MkdirAll(convertOutputDir, 0750); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	outName := convertName
	if outName == "" {
		outName = result.Parsed.Name
	}
	outPath := filepath.Join(convertOutputDir, outName+".md")
	if err := os.WriteFile(outPath, []byte(result.Content), 0600); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	fmt.Printf("✓ Saved to %s (%d tool(s))\n", outPath, len(result.Parsed.Commands))
	return nil
}

// resolveClient builds an llm.Client from the command flags.
//
// If -p is given it uses that provider directly (errors if unknown).
// If -p is omitted it walks providerDefaults and uses the first one
// that has a non-empty key in the vault.
// Returns the client and the resolved provider name (useful for logging).
func resolveClient(ctx context.Context, v vault.Vault) (llm.Client, string, error) {
	if convertProvider != "" {
		client, err := buildClient(v, convertProvider, convertModel, convertVaultKey)
		return client, convertProvider, err
	}

	// Auto-detect: walk providers in preference order.
	for _, pd := range providerDefaults {
		keyPath := pd.vaultKey
		if convertVaultKey != "" {
			keyPath = convertVaultKey // honour explicit --vault-key even without -p
		}

		key, err := v.GetSecret(ctx, keyPath)
		if err != nil || strings.TrimSpace(key) == "" {
			continue
		}

		model := pd.model
		if convertModel != "" {
			model = convertModel // honour -m even during auto-detect
		}

		client, err := buildClient(v, pd.name, model, keyPath)
		if err != nil {
			logger.Log.Warn("auto-detect: failed to build client",
				"provider", pd.name, "error", err)
			continue
		}

		return client, pd.name, nil
	}

	return nil, "", fmt.Errorf(
		"no LLM provider available; pass -p or set an API key in the vault for one of: anthropic, openai, gemini",
	)
}

// buildClient constructs the concrete llm.Client for a named provider,
// filling in model and vault key defaults when not explicitly set.
func buildClient(v vault.Vault, provider, model, vaultKey string) (llm.Client, error) {
	for _, pd := range providerDefaults {
		if pd.name == provider {
			if model == "" {
				model = pd.model
			}
			if vaultKey == "" {
				vaultKey = pd.vaultKey
			}
			break
		}
	}

	cfg := llm.ProviderConfig{
		Model:   model,
		Vault:   v,
		KeyPath: vaultKey,
	}

	switch provider {
	case "anthropic":
		return anthropic.NewAnthropicClient(cfg)
	case "openai":
		return openai.NewOpenAIClient(cfg)
	case "gemini":
		return gemini.NewGeminiClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported provider %q — valid options: anthropic, openai, gemini", provider)
	}
}
