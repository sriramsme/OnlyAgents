package cmdutil

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/skills/cli"
)

// ── Registry ──────────────────────────────────────────────────────────────────

// SkillRegistry loads all skill configs from the skills dir.
func SkillRegistry(skillsDir string) ([]config.SkillConfig, error) {
	entries, err := filepath.Glob(filepath.Join(skillsDir, "*.md"))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", skillsDir, err)
	}
	var skills []config.SkillConfig
	for _, path := range entries {
		data, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			fmt.Fprintf(os.Stderr, StyleYellow.Render("  ! could not read %s: %v\n"), path, err)
			continue
		}
		s, err := parseSkillFile(path, data)
		if err != nil {
			fmt.Fprintf(os.Stderr, StyleYellow.Render("  ! could not parse %s: %v\n"), path, err)
			continue
		}
		skills = append(skills, s)
	}
	return skills, nil
}

func parseSkillFile(path string, data []byte) (config.SkillConfig, error) {
	content := string(data)

	// No frontmatter — treat whole file as content, derive name from filename
	if !strings.HasPrefix(content, "---\n") {
		return config.SkillConfig{
			Name:    strings.TrimSuffix(filepath.Base(path), ".md"),
			Enabled: true,
			Type:    "cli",
		}, nil
	}

	// Extract frontmatter block
	end := strings.Index(content[4:], "\n---")
	if end == -1 {
		return config.SkillConfig{}, fmt.Errorf("unclosed frontmatter")
	}
	frontmatter := content[4 : end+4]

	var s config.SkillConfig
	if err := yaml.Unmarshal([]byte(frontmatter), &s); err != nil {
		return config.SkillConfig{}, fmt.Errorf("parse frontmatter: %w", err)
	}

	// Fallback name to filename if not set in frontmatter
	if s.Name == "" {
		s.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	return s, nil
} // ── Queries ───────────────────────────────────────────────────────────────────

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
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return err
	}
	content := string(data)

	// Update enabled field in frontmatter via simple string replacement
	// This preserves comments and formatting in the rest of the file
	old := fmt.Sprintf("enabled: %v", !enabled)
	new := fmt.Sprintf("enabled: %v", enabled)
	if !strings.Contains(content, old) {
		return fmt.Errorf("enabled field not found in frontmatter of %s", name)
	}
	updated := strings.Replace(content, old, new, 1)
	return os.WriteFile(path, []byte(updated), 0o600) //nolint:gosec
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

func SkillSetAccessLevel(skillsDir, name, level string) error {
	path := SkillConfigPath(skillsDir, name)
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return err
	}
	content := string(data)
	// Replace existing access_level line or insert after enabled line
	if strings.Contains(content, "access_level:") {
		old := regexp.MustCompile(`access_level:\s*\S+`)
		content = old.ReplaceAllString(content, "access_level: "+level)
	} else {
		content = strings.Replace(content, "enabled:", "access_level: "+level+"\nenabled:", 1)
	}
	return os.WriteFile(path, []byte(content), 0o600) //nolint:gosec
}

// ── Display ───────────────────────────────────────────────────────────────────

// SkillSummaryLine returns a single-line summary for table output.
func SkillSummaryLine(s config.SkillConfig) string {
	return fmt.Sprintf("%-20s %s", s.Name, EnabledLabel(s.Enabled))
}

func SkillConfigPath(skillsDir, name string) string {
	return filepath.Join(skillsDir, name+".md")
}

// SkillWithTools holds frontmatter metadata plus parsed commands.
type SkillWithTools struct {
	config.SkillConfig
	Commands []*cli.Command
}

// LoadSkillWithTools parses a skill's full .md file including tool sections.
func LoadSkillWithTools(skillsDir, name string) (*SkillWithTools, error) {
	path := SkillConfigPath(skillsDir, name)
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read skill %s: %w", name, err)
	}
	parsed, err := cli.ParseSKILLMD(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse skill %s: %w", name, err)
	}
	skills, err := SkillRegistry(skillsDir)
	if err != nil {
		return nil, err
	}
	s, err := FindSkill(skills, name)
	if err != nil {
		return nil, err
	}
	return &SkillWithTools{
		SkillConfig: s,
		Commands:    parsed.Commands,
	}, nil
}
