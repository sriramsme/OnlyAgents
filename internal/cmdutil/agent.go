package cmdutil

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sriramsme/OnlyAgents/internal/config"
)

// ── Registry ──────────────────────────────────────────────────────────────────

// AgentRegistry loads all agent configs from the agents dir.
func AgentRegistry(agentsDir string) ([]config.AgentConfig, error) {
	agents, err := LoadDir[config.AgentConfig](agentsDir)
	if err != nil {
		return nil, fmt.Errorf("agent registry: %w", err)
	}
	return agents, nil
}

// ── Queries ───────────────────────────────────────────────────────────────────

// EnabledAgents returns only agents with Enabled = true.
func EnabledAgents(agents []config.AgentConfig) []config.AgentConfig {
	var out []config.AgentConfig
	for _, a := range agents {
		if a.Enabled {
			out = append(out, a)
		}
	}
	return out
}

// FindAgent returns the first agent with the given ID, or an error.
func FindAgent(agents []config.AgentConfig, id string) (config.AgentConfig, error) {
	for _, a := range agents {
		if a.ID == id {
			return a, nil
		}
	}
	return config.AgentConfig{}, fmt.Errorf("agent %q not found", id)
}

// ── Mutations ─────────────────────────────────────────────────────────────────

// AgentSetEnabled sets the enabled flag on an agent config file.
func AgentSetEnabled(agentsDir, id string, enabled bool) error {
	path := filepath.Join(agentsDir, id+".yaml")
	var raw map[string]any
	if err := ReadYAML(path, &raw); err != nil {
		return fmt.Errorf("read agent %s: %w", id, err)
	}
	raw["enabled"] = enabled
	if err := WriteYAML(path, raw); err != nil {
		return fmt.Errorf("write agent %s: %w", id, err)
	}
	return nil
}

// AgentSetLLM updates the llm block of an agent config file.
func AgentSetLLM(agentsDir, id, provider, model, vaultPath string) error {
	path := filepath.Join(agentsDir, id+".yaml")
	var raw map[string]any
	if err := ReadYAML(path, &raw); err != nil {
		return fmt.Errorf("read agent %s: %w", id, err)
	}
	raw["llm"] = map[string]any{
		"provider":      provider,
		"model":         model,
		"api_key_vault": vaultPath,
	}
	if err := WriteYAML(path, raw); err != nil {
		return fmt.Errorf("write agent %s: %w", id, err)
	}
	return nil
}

// ── Validation ────────────────────────────────────────────────────────────────

// ValidateAgents checks for common config problems.
// Returns a slice of human-readable error strings (not Go errors — these are
// display messages for the sanitize command).
func ValidateAgents(agents []config.AgentConfig) []string {
	var issues []string
	seenIDs := map[string]int{}
	executiveCount := 0
	generalCount := 0

	for i, a := range agents {
		prefix := fmt.Sprintf("agent[%d] %q", i, a.ID)

		if a.ID == "" {
			issues = append(issues, prefix+": missing id")
		}
		seenIDs[a.ID]++
		if seenIDs[a.ID] > 1 {
			issues = append(issues, fmt.Sprintf("duplicate agent id %q", a.ID))
		}
		if a.LLM.Provider == "" {
			issues = append(issues, prefix+": llm.provider is empty")
		}
		if a.LLM.Model == "" {
			issues = append(issues, prefix+": llm.model is empty")
		}
		if a.LLM.APIKeyVault == "" {
			issues = append(issues, prefix+": llm.api_key_vault is empty")
		}
		if a.IsExecutive {
			executiveCount++
		}
		if a.IsGeneral {
			generalCount++
		}
	}
	if executiveCount == 0 {
		issues = append(issues, "no executive agent defined (is_executive: true)")
	}
	if executiveCount > 1 {
		issues = append(issues, fmt.Sprintf("multiple executive agents defined (%d) — only one allowed", executiveCount))
	}
	if generalCount > 1 {
		issues = append(issues, fmt.Sprintf("multiple general agents defined (%d) — only one allowed", generalCount))
	}

	return issues
}

// ── Display ───────────────────────────────────────────────────────────────────

// AgentSummaryLine returns a single-line summary of an agent for table output.
func AgentSummaryLine(a config.AgentConfig) string {
	role := "sub-agent"
	if a.IsExecutive {
		role = "executive"
	} else if a.IsGeneral {
		role = "general"
	}
	return fmt.Sprintf("%-16s %-12s %-10s %-12s %s/%s",
		a.ID,
		a.Name,
		role,
		EnabledLabel(a.Enabled),
		a.LLM.Provider,
		a.LLM.Model,
	)
}

// AgentConfigPath returns the expected path for an agent config file.
func AgentConfigPath(agentsDir, id string) string {
	return filepath.Join(agentsDir, id+".yaml")
}

// AgentExists checks whether a config file exists for the given agent ID.
func AgentExists(agentsDir, id string) bool {
	_, err := os.Stat(AgentConfigPath(agentsDir, id))
	return err == nil
}
