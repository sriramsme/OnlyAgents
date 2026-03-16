package cmdutil

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/sriramsme/OnlyAgents/internal/config"
)

// ── Registry ──────────────────────────────────────────────────────────────────

// ChannelRegistry loads all channel configs from the channels dir.
func ChannelRegistry(channelsDir string) ([]config.Channel, error) {
	channels, err := LoadDir[config.Channel](channelsDir)
	if err != nil {
		return nil, fmt.Errorf("channel registry: %w", err)
	}
	return channels, nil
}

// ── Queries ───────────────────────────────────────────────────────────────────

// EnabledChannels returns only channels with Enabled = true.
func EnabledChannels(channels []config.Channel) []config.Channel {
	var out []config.Channel
	for _, c := range channels {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out
}

func FindChannelByPlatform(channels []config.Channel, platform string) (config.Channel, error) {
	for _, c := range channels {
		if c.Platform == platform {
			return c, nil
		}
	}
	return config.Channel{}, fmt.Errorf("channel with platform %q not found", platform)
}

func SetupChannel(cfg config.Channel, envPath, channelsDir string) error {
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
	return ChannelEnable(channelsDir, cfg, true)
}

// ── Mutations ─────────────────────────────────────────────────────────────────

// ChannelSetEnabled sets enabled on a channel config file.
func ChannelSetEnabled(channelsDir, name string, enabled bool) error {
	path := ChannelConfigPath(channelsDir, name)
	var raw map[string]any
	if err := ReadYAML(path, &raw); err != nil {
		return fmt.Errorf("read channel %s: %w", name, err)
	}
	raw["enabled"] = enabled
	if err := WriteYAML(path, raw); err != nil {
		return fmt.Errorf("write channel %s: %w", name, err)
	}
	return nil
}

// ChannelSetVaultKey sets a vault key path on a channel config.
// keyField is the YAML field name, e.g. "bot_token_vault".
func ChannelSetVaultKey(channelsDir, name, keyField, vaultPath string) error {
	path := ChannelConfigPath(channelsDir, name)
	var raw map[string]any
	if err := ReadYAML(path, &raw); err != nil {
		return fmt.Errorf("read channel %s: %w", name, err)
	}
	raw[keyField] = vaultPath
	raw["enabled"] = true
	if err := WriteYAML(path, raw); err != nil {
		return fmt.Errorf("write channel %s: %w", name, err)
	}
	return nil
}

func ChannelEnable(channelsDir string, cfg config.Channel, enabled bool) error {
	path := ChannelConfigPath(channelsDir, cfg.Platform)
	var raw map[string]any
	if err := ReadYAML(path, &raw); err != nil {
		return err
	}
	raw["enabled"] = enabled
	return WriteYAML(path, raw)
}

// ── Validation ────────────────────────────────────────────────────────────────

// ValidateChannels checks for common channel config problems.
func ValidateChannels(channels []config.Channel) []string {
	var issues []string
	seenNames := map[string]int{}

	for i, c := range channels {
		prefix := fmt.Sprintf("channel[%d] %q", i, c.Name)

		if c.Name == "" {
			issues = append(issues, fmt.Sprintf("channel[%d]: missing name (derived from filename)", i))
		}
		seenNames[c.Name]++
		if seenNames[c.Name] > 1 {
			issues = append(issues, fmt.Sprintf("duplicate channel name %q", c.Name))
		}
		if c.Platform == "" {
			issues = append(issues, prefix+": platform is empty")
		}
	}

	enabledCount := len(EnabledChannels(channels))
	if enabledCount == 0 {
		issues = append(issues, "no channels enabled — agents cannot receive or send messages")
	}

	return issues
}

// ── Display ───────────────────────────────────────────────────────────────────

// ChannelSummaryLine returns a single-line summary for table output.
func ChannelSummaryLine(c config.Channel) string {
	return fmt.Sprintf("%-16s %-12s %s",
		c.Name,
		c.Platform,
		EnabledLabel(c.Enabled),
	)
}

// ChannelConfigPath returns the expected path for a channel config file.
func ChannelConfigPath(channelsDir, name string) string {
	return filepath.Join(channelsDir, name+".yaml")
}
