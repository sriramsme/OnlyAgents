package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/auth"
	"github.com/sriramsme/OnlyAgents/internal/cmdutil"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/internal/paths"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"gopkg.in/yaml.v3"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard — run this first",
	Long: `Walks you through configuring OnlyAgents step by step.
Safe to re-run — already configured steps can be skipped.`,
	RunE: runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	ctx := cmdutil.NewSetupContext()

	steps := []cmdutil.SetupStep{
		&bootstrapStep{},
		&userIdentityStep{},
		&vaultStep{},
		&llmStep{},
		&channelStep{},
		&authSetupStep{},
	}

	return cmdutil.NewSetupRunner(steps, ctx).Run()
}

// ── Step 1: Bootstrap ─────────────────────────────────────────────────────────

type bootstrapStep struct{}

func (s *bootstrapStep) Name() string { return "Bootstrap" }
func (s *bootstrapStep) Description() string {
	return "Create ~/.onlyagents/ directory structure and seed defaults"
}

func (s *bootstrapStep) IsDone(ctx *cmdutil.SetupContext) bool {
	if ctx.Paths == nil {
		return false
	}
	_, err := os.Stat(ctx.Paths.Home)
	return err == nil
}

func (s *bootstrapStep) Run(ctx *cmdutil.SetupContext) error {
	paths, err := paths.Init()
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	ctx.Paths = paths
	cmdutil.Success("created %s", paths.Home)
	return nil
}

// ── Step 2: User Identity ─────────────────────────────────────────────────────

type userIdentityStep struct{}

func (s *userIdentityStep) Name() string        { return "User Profile" }
func (s *userIdentityStep) Description() string { return "Tell agents who you are" }

func (s *userIdentityStep) IsDone(ctx *cmdutil.SetupContext) bool {
	if ctx.Paths == nil {
		return false
	}
	data, err := os.ReadFile(ctx.Paths.UserPath)
	if err != nil {
		return false
	}
	var u config.User
	if err := yaml.Unmarshal(data, &u); err != nil {
		return false
	}
	return strings.TrimSpace(u.Identity.Name) != ""
}

func (s *userIdentityStep) Run(ctx *cmdutil.SetupContext) error {
	cmdutil.Hint(
		"Agents use this to personalise responses.",
		"You can edit ~/.onlyagents/user.yaml directly for more detail.",
	)

	var (
		name          = ctx.UserName
		preferredName = ctx.UserPreferredName
		role          = ctx.UserRole
		tz            string
	)

	// Default timezone from system
	if tz == "" {
		zoneName, _ := time.Now().Zone()

		tz = zoneName
		if data, err := os.ReadFile("/etc/timezone"); err == nil {
			tz = strings.TrimSpace(string(data))
		}
	}

	err := cmdutil.RunForm(
		huh.NewGroup(
			cmdutil.RequiredInput("Your name", "Ada Lovelace", &name),
			cmdutil.InputField("Preferred name (how agents address you)", "boss", &preferredName),
			cmdutil.InputField("Role / title", "Founder & Engineer", &role),
			cmdutil.RequiredInput("Timezone", "America/New_York", &tz),
		),
	)
	if err != nil {
		return err
	}

	ctx.UserName = name
	ctx.UserPreferredName = preferredName
	ctx.UserRole = role
	ctx.UserTimezone = tz

	// Load existing or build fresh User
	cfg := config.User{}
	if data, err := os.ReadFile(ctx.Paths.UserPath); err == nil {
		_ = yaml.Unmarshal(data, &cfg) // nolint:errcheck
	}
	cfg.Identity.Name = name
	cfg.Identity.PreferredName = preferredName
	cfg.Identity.Role = role
	cfg.Identity.Timezone = tz

	return cmdutil.WriteYAML(ctx.Paths.UserPath, cfg)
}

// ── Step 3: Vault ─────────────────────────────────────────────────────────────

type vaultStep struct{}

func (s *vaultStep) Name() string        { return "Vault" }
func (s *vaultStep) Description() string { return "Configure how secrets (API keys) are stored" }

func (s *vaultStep) IsDone(ctx *cmdutil.SetupContext) bool {
	if ctx.Paths == nil {
		return false
	}
	_, err := os.Stat(ctx.Paths.VaultPath)
	return err == nil
}

func (s *vaultStep) Run(ctx *cmdutil.SetupContext) error {
	cmdutil.Hint(
		"OnlyAgents never hardcodes secrets.",
		"The simplest setup: a .env file in ~/.onlyagents/",
		"",
		"Keys are referenced in agent configs as vault paths:",
		"  ANTHROPIC_API_KEY  →  anthropic/api_key",
		"  OPENAI_API_KEY     →  openai/api_key",
		"  TELEGRAM_BOT_TOKEN →  telegram/bot_token",
	)

	vaultCfg := fmt.Sprintf(`type: env
dotenv_path: '%s'
enable_cache: true
audit_log: false
`, ctx.Paths.EnvPath)

	if err := os.WriteFile(ctx.Paths.VaultPath, []byte(vaultCfg), 0o600); err != nil {
		return fmt.Errorf("write vault.yaml: %w", err)
	}

	// Create .env if it doesn't exist
	if _, err := os.Stat(ctx.Paths.EnvPath); os.IsNotExist(err) {
		header := "# OnlyAgents secrets\n# Add your API keys below\n\n"
		if err := os.WriteFile(ctx.Paths.EnvPath, []byte(header), 0o600); err != nil {
			return fmt.Errorf("create .env: %w", err)
		}
	}

	ctx.EnvFilePath = ctx.Paths.EnvPath
	cmdutil.Success("vault.yaml written")
	cmdutil.Success(".env created at %s", ctx.Paths.EnvPath)
	return nil
}

// ── Step 4: LLM ───────────────────────────────────────────────────────────────
type llmStep struct{}

func (s *llmStep) Name() string        { return "LLM Providers" }
func (s *llmStep) Description() string { return "Configure which LLM each agent uses" }

func (s *llmStep) IsDone(ctx *cmdutil.SetupContext) bool {
	return len(ctx.LLMChoices) > 0
}

func (s *llmStep) Run(ctx *cmdutil.SetupContext) error {
	cmdutil.Hint(
		"Each agent can use a different provider and model.",
		"You can use the same API key for all agents, or different ones.",
		"Keys will be written to your .env file.",
	)

	defaultAgentSlots, err := cmdutil.AgentRegistry(ctx.Paths.Agents)
	llmProviders := cmdutil.ProviderOptions()
	if err != nil {
		return err
	}
	// Collect unique providers the user wants to use
	usedProviders := map[string]string{} // provider → apiKey

	for _, slot := range defaultAgentSlots {
		llmProviderModels, err := cmdutil.ModelOptions(slot.LLM.Provider)
		if err != nil {
			return err
		}
		llmEnvVar := cmdutil.ProviderEnvVar(slot.LLM.Provider)
		llmVaultPath := cmdutil.ProviderVaultPath(slot.LLM.Provider)

		cmdutil.Section(fmt.Sprintf("Agent: %s", slot.Name))
		cmdutil.Info("%s", slot.Description)
		fmt.Println()

		var provider, model, apiKey string

		// Provider select
		err = cmdutil.RunForm(
			huh.NewGroup(
				cmdutil.SelectField("Provider", llmProviders, &provider),
			),
		)
		if err != nil {
			return err
		}

		// Model select — depends on provider
		err = cmdutil.RunForm(
			huh.NewGroup(
				cmdutil.SelectField("Model", llmProviderModels, &model),
			),
		)
		if err != nil {
			return err
		}

		// API key — reuse if already entered for this provider
		if existing, ok := usedProviders[provider]; ok {
			apiKey = existing
			cmdutil.Info("reusing %s from earlier", llmEnvVar)
		} else {
			err = cmdutil.RunForm(
				huh.NewGroup(
					cmdutil.SecretInput(fmt.Sprintf("%s (env: %s)", provider, llmEnvVar), &apiKey),
				),
			)
			if err != nil {
				return err
			}
			usedProviders[provider] = apiKey
		}

		ctx.LLMChoices[slot.ID] = cmdutil.LLMChoice{
			Provider:   provider,
			Model:      model,
			APIKeyPath: llmVaultPath,
			EnvVarName: llmEnvVar,
		}
	}

	// Write all keys to .env
	envContent, err := os.ReadFile(ctx.Paths.EnvPath)
	if err != nil {
		return err
	}
	existing := string(envContent)
	for provider, key := range usedProviders {
		envVar := cmdutil.ProviderEnvVar(provider)
		line := fmt.Sprintf("%s=%s", envVar, key)
		if !strings.Contains(existing, envVar) {
			existing += line + "\n"
		}
	}
	if err := os.WriteFile(ctx.Paths.EnvPath, []byte(existing), 0o600); err != nil { //nolint:gosec
		return fmt.Errorf("write .env: %w", err)
	}

	// Update agent config files
	for _, slot := range defaultAgentSlots {
		choice := ctx.LLMChoices[slot.ID]
		agentPath := filepath.Join(ctx.Paths.Agents, slot.ID+".yaml")
		if err := updateAgentLLM(agentPath, choice); err != nil {
			cmdutil.Warn("could not update %s.yaml: %v", slot.ID, err)
		}
	}

	return nil
}

// updateAgentLLM reads an existing agent YAML, updates the llm block, writes back.
func updateAgentLLM(path string, choice cmdutil.LLMChoice) error {
	clean := filepath.Clean(path)
	data, err := os.ReadFile(clean) //nolint:gosec
	if err != nil {
		return err
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}

	raw["llm"] = map[string]any{
		"provider":     choice.Provider,
		"model":        choice.Model,
		"api_key_path": choice.APIKeyPath,
	}

	return cmdutil.WriteYAML(path, raw)
}

// ── Step 5: Channel ───────────────────────────────────────────────────────────

type channelStep struct{}

func (s *channelStep) Name() string                          { return "Channel" }
func (s *channelStep) Description() string                   { return "Choose how you'll talk to your agents" }
func (s *channelStep) IsDone(ctx *cmdutil.SetupContext) bool { return ctx.ChannelChoice != "" }
func (s *channelStep) Run(ctx *cmdutil.SetupContext) error {
	// Offer skip — OAChannel always works without any setup
	skip := false
	if err := cmdutil.RunForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Set up a channel now?").
				Description("OAChannel (built-in web UI) is always available without setup.\nYou can add other channels later with `onlyagents channel setup <platform>`.").
				Affirmative("Yes, set one up").
				Negative("Skip, I'll use the web UI").
				Value(&skip),
		),
	); err != nil {
		return err
	}
	if !skip {
		ctx.ChannelChoice = "oachannel"
		cmdutil.Success("using built-in OAChannel — run 'onlyagents server start' and open http://localhost:19965")
		return nil
	}

	configs, err := cmdutil.ChannelRegistry(ctx.Paths.Channels)
	if err != nil {
		return fmt.Errorf("load channels: %w", err)
	}

	// Exclude oachannel from the list — it needs no setup
	var setupable []channels.Config
	for _, c := range configs {
		if c.Platform != "oachannel" {
			setupable = append(setupable, c)
		}
	}

	opts := make([]huh.Option[string], len(setupable))
	for i, cfg := range setupable {
		label := fmt.Sprintf("%-20s %s", cfg.Name, cmdutil.Truncate(cfg.Description, 48))
		opts[i] = huh.NewOption(label, cfg.Platform)
	}

	var choice string
	if err := cmdutil.RunForm(
		huh.NewGroup(
			cmdutil.SelectField("Which channel?", opts, &choice),
		),
	); err != nil {
		return err
	}

	cfg, err := cmdutil.FindChannelByPlatform(configs, choice)
	if err != nil {
		return err
	}

	if cfg.Instructions != "" {
		cmdutil.Hint(cfg.Instructions)
	}

	// Collect secrets — driven entirely by vault_paths in the channel YAML
	for _, vp := range cfg.VaultPaths {
		var value string
		if err := cmdutil.RunForm(huh.NewGroup(
			cmdutil.SecretInput(vp.Prompt, &value),
		)); err != nil {
			return err
		}
		if err := cmdutil.AppendEnvVar(ctx.Paths.EnvPath, vp.Path, value); err != nil {
			return err
		}
	}

	if err := cmdutil.ChannelEnable(ctx.Paths.Channels, cfg, true); err != nil {
		cmdutil.Warn("could not enable %s: %v", cfg.Name, err)
	}

	ctx.ChannelChoice = choice
	cmdutil.Success("%s configured", cfg.Name)
	return nil
}

// ── Step 6: Auth ──────────────────────────────────────────────────────────────

type authSetupStep struct{}

func (s *authSetupStep) Name() string        { return "Auth" }
func (s *authSetupStep) Description() string { return "Set your server login password" }

func (s *authSetupStep) IsDone(ctx *cmdutil.SetupContext) bool {
	if ctx.Paths == nil {
		return false
	}
	authPath := filepath.Join(ctx.Paths.Home, "auth.yaml")
	_, err := os.Stat(authPath)
	return err == nil
}

func (s *authSetupStep) Run(ctx *cmdutil.SetupContext) error {
	cmdutil.Hint(
		"This password protects the OnlyAgents web interface and API.",
		"Username is 'admin' by default.",
	)

	var username, password, confirm string

	err := cmdutil.RunForm(
		huh.NewGroup(
			cmdutil.RequiredInput("Admin username", "john-admin", &username).
				Validate(func(s string) error {
					if len(s) < 3 {
						return fmt.Errorf("must be at least 3 characters")
					}
					return nil
				}),

			cmdutil.SecretInput("Password (min 8 characters)", &password).
				Validate(func(s string) error {
					if len(s) < 8 {
						return fmt.Errorf("must be at least 8 characters")
					}
					return nil
				}),

			cmdutil.SecretInput("Confirm password", &confirm).
				Validate(func(s string) error {
					if s != password {
						return fmt.Errorf("passwords do not match")
					}
					return nil
				}),
		),
	)
	if err != nil {
		return err
	}

	if err = auth.CreateUser(ctx.Paths.Home, username, password); err != nil {
		return err
	}

	// Generate and store API key for programmatic access
	apiKey, err := cmdutil.GenerateAPIKey()
	if err != nil {
		return fmt.Errorf("generate api key: %w", err)
	}
	if err := cmdutil.AppendEnvVar(ctx.EnvFilePath, "server/api_key", apiKey); err != nil {
		return fmt.Errorf("store api key: %w", err)
	}

	cmdutil.Success("auth configured — username: %s", username)
	cmdutil.Info("API key saved to vault — for programmatic access:")
	cmdutil.Hint("  %s", apiKey)
	cmdutil.Hint("This key won't be shown again. Find it in ~/.onlyagents/.env if needed.")
	return nil
}
