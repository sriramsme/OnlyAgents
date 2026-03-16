package cmdutil

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/sriramsme/OnlyAgents/internal/config"
)

// ── Registry ──────────────────────────────────────────────────────────────────

// ConnectorRegistry loads all connector configs from the connectors dir.
func ConnectorRegistry(connectorsDir string) ([]config.Connector, error) {
	connectors, err := LoadDir[config.Connector](connectorsDir)
	if err != nil {
		return nil, fmt.Errorf("connector registry: %w", err)
	}
	return connectors, nil
}

// ── Queries ───────────────────────────────────────────────────────────────────

// EnabledConnectors returns only connectors with Enabled = true.
func EnabledConnectors(connectors []config.Connector) []config.Connector {
	var out []config.Connector
	for _, c := range connectors {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out
}

// FindConnector returns the first connector matching name.
func FindConnector(connectors []config.Connector, name string) (config.Connector, error) {
	for _, c := range connectors {
		if c.Name == name {
			return c, nil
		}
	}
	return config.Connector{}, fmt.Errorf("connector %q not found", name)
}

func SetupConnector(cfg config.Connector, envPath, connectorsDir string) error {
	if cfg.Instructions != "" {
		Hint(cfg.Instructions)
	}
	for _, vp := range cfg.VaultPaths {
		var value string
		if err := RunForm(huh.NewGroup(SecretInput(vp.Prompt, &value))); err != nil {
			return err
		}
		if err := AppendEnvVar(envPath, vp.Path, value); err != nil {
			return err
		}
	}
	return ConnectorSetEnabled(connectorsDir, cfg.Name, true)
}

// ── Mutations ─────────────────────────────────────────────────────────────────

// ConnectorSetEnabled sets the enabled flag on a connector config file.
func ConnectorSetEnabled(connectorsDir, name string, enabled bool) error {
	path := ConnectorConfigPath(connectorsDir, name)
	var raw map[string]any
	if err := ReadYAML(path, &raw); err != nil {
		return fmt.Errorf("read connector %s: %w", name, err)
	}
	raw["enabled"] = enabled
	if err := WriteYAML(path, raw); err != nil {
		return fmt.Errorf("write connector %s: %w", name, err)
	}
	return nil
}

// ── Validation ────────────────────────────────────────────────────────────────

// ValidateConnectors checks for common connector config problems.
func ValidateConnectors(connectors []config.Connector) []string {
	var issues []string
	seenNames := map[string]int{}

	for i, c := range connectors {
		prefix := fmt.Sprintf("connector[%d] %q", i, c.Name)

		if c.Name == "" {
			issues = append(issues, fmt.Sprintf("connector[%d]: missing name", i))
		}
		seenNames[c.Name]++
		if seenNames[c.Name] > 1 {
			issues = append(issues, fmt.Sprintf("duplicate connector name %q", c.Name))
		}
		if c.Type == "" {
			issues = append(issues, prefix+": type is empty")
		}
	}
	return issues
}

// ── Display ───────────────────────────────────────────────────────────────────

// ConnectorSummaryLine returns a single-line summary for table output.
func ConnectorSummaryLine(c config.Connector) string {
	return fmt.Sprintf("%-20s %-12s %s",
		c.Name,
		c.Type,
		EnabledLabel(c.Enabled),
	)
}

// ConnectorConfigPath returns the expected path for a connector config file.
func ConnectorConfigPath(connectorsDir, name string) string {
	return filepath.Join(connectorsDir, name+".yaml")
}
