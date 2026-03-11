package cmdutil

import (
	"fmt"
	"path/filepath"

	"github.com/sriramsme/OnlyAgents/internal/config"
)

// ── Registry ──────────────────────────────────────────────────────────────────

// SkillRegistry loads all skill configs from the skills dir.
func SkillRegistry(skillsDir string) ([]config.SkillConfig, error) {
	skills, err := LoadDir[config.SkillConfig](skillsDir)
	if err != nil {
		return nil, fmt.Errorf("skill registry: %w", err)
	}
	return skills, nil
}

// ── Queries ───────────────────────────────────────────────────────────────────

// EnabledSkills returns only skills with Enabled = true.
func EnabledSkills(skills []config.SkillConfig) []config.SkillConfig {
	var out []config.SkillConfig
	for _, s := range skills {
		if s.Enabled {
			out = append(out, s)
		}
	}
	return out
}

// FindSkill returns the first skill matching name.
func FindSkill(skills []config.SkillConfig, name string) (config.SkillConfig, error) {
	for _, s := range skills {
		if s.Name == name {
			return s, nil
		}
	}
	return config.SkillConfig{}, fmt.Errorf("skill %q not found", name)
}

// ── Mutations ─────────────────────────────────────────────────────────────────

// SkillSetEnabled sets the enabled flag on a skill config file.
func SkillSetEnabled(skillsDir, name string, enabled bool) error {
	path := SkillConfigPath(skillsDir, name)
	var raw map[string]any
	if err := ReadYAML(path, &raw); err != nil {
		return fmt.Errorf("read skill %s: %w", name, err)
	}
	raw["enabled"] = enabled
	if err := WriteYAML(path, raw); err != nil {
		return fmt.Errorf("write skill %s: %w", name, err)
	}
	return nil
}

// ── Validation ────────────────────────────────────────────────────────────────

// ValidateSkills checks for common skill config problems.
func ValidateSkills(skills []config.SkillConfig) []string {
	var issues []string
	seenNames := map[string]int{}

	for i, s := range skills {
		prefix := fmt.Sprintf("skill[%d] %q", i, s.Name)

		if s.Name == "" {
			issues = append(issues, fmt.Sprintf("skill[%d]: missing name", i))
		}
		seenNames[s.Name]++
		if seenNames[s.Name] > 1 {
			issues = append(issues, fmt.Sprintf("duplicate skill name %q", s.Name))
		}
		_ = prefix
	}
	return issues
}

// ── Display ───────────────────────────────────────────────────────────────────

// SkillSummaryLine returns a single-line summary for table output.
func SkillSummaryLine(s config.SkillConfig) string {
	return fmt.Sprintf("%-20s %s", s.Name, EnabledLabel(s.Enabled))
}

// SkillConfigPath returns the expected path for a skill config file.
func SkillConfigPath(skillsDir, name string) string {
	return filepath.Join(skillsDir, name+".yaml")
}
