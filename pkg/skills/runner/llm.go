// pkg/skills/runner/llm.go
package runner

import (
	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
)

// RegisterLLMFlags adds all LLM auth and config flags to root.
func RegisterLLMFlags(root *cobra.Command) {
	f := root.PersistentFlags()
	f.String("provider", "openai", "LLM provider (anthropic, openai, gemini)")
	f.String("model", "", "LLM model (defaults to provider default)")
	// auth — choose one
	f.String("api-key", "", "API key (direct value)")
	f.String("api-key-name", "", "Env var name to read API key from (e.g. MY_ANTHROPIC_KEY)")
	f.String("env-path", "", "Path to .env file for API key resolution")
	// optional
	f.String("base-url", "", "Custom base URL for provider API")
}

// BuildLLMClientFromCmd constructs an llm.Client from parsed cobra flags.
func BuildLLMClientFromCmd(root *cobra.Command) (llm.Client, error) {
	f := root.PersistentFlags()

	var err error
	var provider, model, apiKey, apiKeyName, envPath, baseURL string

	if provider, err = f.GetString("provider"); err != nil {
		return nil, err
	}
	if model, err = f.GetString("model"); err != nil {
		return nil, err
	}
	if apiKey, err = f.GetString("api-key"); err != nil {
		return nil, err
	}
	if apiKeyName, err = f.GetString("api-key-name"); err != nil {
		return nil, err
	}
	if envPath, err = f.GetString("env-path"); err != nil {
		return nil, err
	}
	if baseURL, err = f.GetString("base-url"); err != nil {
		return nil, err
	}
	if model == "" {
		model = llm.ProviderDefaultModel(provider)
	}

	return llm.New(llm.Config{
		Provider:   provider,
		Model:      model,
		APIKey:     apiKey,
		APIKeyName: apiKeyName,
		EnvPath:    envPath,
		BaseURL:    baseURL,
		// Vault not supported via CLI — use env vars instead
	})
}

// LLMSetup returns a setup func for Run() for LLM-backed skills.
func LLMSetup(build func(llm.Client) (skills.Skill, error)) func(*cobra.Command) (skills.Skill, error) {
	return func(root *cobra.Command) (skills.Skill, error) {
		client, err := BuildLLMClientFromCmd(root)
		if err != nil {
			return nil, err
		}
		return build(client)
	}
}
