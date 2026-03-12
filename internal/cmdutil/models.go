package cmdutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/anthropic"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/gemini"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/openai"
)

// ── Registries ────────────────────────────────────────────────────────────────

// LLMRegistry returns the model registry for a given provider name.
// provider = "all" | "anthropic" | "openai" | "gemini"
func LLMRegistry(provider string) (map[string]llm.ModelCapabilities, error) {
	switch provider {
	case "all":
		return AllLLMRegistries(), nil
	case "anthropic":
		return anthropic.ModelRegistry, nil
	case "openai":
		return openai.ModelRegistry, nil
	case "gemini":
		return gemini.ModelRegistry, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

// AllLLMRegistries merges all provider registries into one map.
func AllLLMRegistries() map[string]llm.ModelCapabilities {
	all := make(map[string]llm.ModelCapabilities)
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

// KnownProviders returns the list of supported provider names.
func KnownProviders() []string {
	return []string{"anthropic", "openai", "gemini"}
}

// ── huh option builders ───────────────────────────────────────────────────────

// ProviderOptions returns huh select options for all known providers.
func ProviderOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("Anthropic (Claude)", "anthropic"),
		huh.NewOption("OpenAI (GPT)", "openai"),
		huh.NewOption("Google Gemini", "gemini"),
	}
}

// ModelOptions returns huh select options for models belonging to provider,
// built dynamically from the registry. Non-deprecated models only.
func ModelOptions(provider string) ([]huh.Option[string], error) {
	registry, err := LLMRegistry(provider)
	if err != nil {
		return nil, err
	}

	infos := llm.GetAllModelsInfo(registry)
	var opts []huh.Option[string]
	for _, info := range infos {
		if info.Capabilities.Deprecated {
			continue
		}
		label := fmt.Sprintf("%-40s %s", info.Name, Truncate(info.Capabilities.Description, 35))
		opts = append(opts, huh.NewOption(label, info.Name))
	}
	if len(opts) == 0 {
		return nil, fmt.Errorf("no models found for provider %s", provider)
	}
	return opts, nil
}

// ── Vault path / env var conventions ─────────────────────────────────────────

// ProviderEnvVar returns the conventional environment variable name for a
// provider's API key. e.g. "anthropic" → "ANTHROPIC_API_KEY"
func ProviderEnvVar(provider string) string {
	m := map[string]string{
		"anthropic": "ANTHROPIC_API_KEY",
		"openai":    "OPENAI_API_KEY",
		"gemini":    "GEMINI_API_KEY",
	}
	if v, ok := m[provider]; ok {
		return v
	}
	return ""
}

// ProviderVaultPath returns the vault key path for a provider's API key.
// e.g. "anthropic" → "anthropic/api_key"
func ProviderVaultPath(provider string) string {
	m := map[string]string{
		"anthropic": "anthropic/api_key",
		"openai":    "openai/api_key",
		"gemini":    "gemini/api_key",
	}
	if v, ok := m[provider]; ok {
		return v
	}
	return ""
}

func ProviderDefaultModel(provider string) string {
	m := map[string]string{
		"anthropic": "claude-sonnet-4-6",
		"openai":    "gpt-4o",
		"gemini":    "gemini-1.5-pro",
	}
	if v, ok := m[provider]; ok {
		return v
	}
	return ""
}

// AutoDetectProvider walks known providers and returns the first one with a
// non-empty vault secret. Returns the provider name, vault key, and ok=true.
func AutoDetectProvider(ctx context.Context, v interface {
	GetSecret(context.Context, string) (string, error)
},
) (provider, vaultKey string, ok bool) {
	for _, p := range KnownProviders() {
		key := ProviderVaultPath(p)
		secret, err := v.GetSecret(ctx, key)
		if err == nil && strings.TrimSpace(secret) != "" {
			return p, key, true
		}
	}
	return "", "", false
}
