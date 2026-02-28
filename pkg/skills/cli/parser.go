package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// ──────────────────────────────────────────────────────────────
// Frontmatter types
// ──────────────────────────────────────────────────────────────

// Frontmatter is the YAML block between the opening and closing --- delimiters.
type Frontmatter struct {
	Name         string       `yaml:"name"`
	Description  string       `yaml:"description"`
	Version      string       `yaml:"version"`
	Capabilities []string     `yaml:"capabilities"`
	Authors      []Author     `yaml:"authors"`
	Homepage     string       `yaml:"homepage"`
	Requires     Requirements `yaml:"requires"`
	Security     SecurityInfo `yaml:"security"`
}

// Author is an optional author entry in the frontmatter.
type Author struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

// Requirements lists external binaries and environment variables the skill needs.
type Requirements struct {
	Bins []string `yaml:"bins"`
	Env  []string `yaml:"env"`
}

// SecurityInfo tracks sanitisation metadata for a skill.
type SecurityInfo struct {
	Sanitized   bool   `yaml:"sanitized"`
	SanitizedAt string `yaml:"sanitized_at"`
	SanitizedBy string `yaml:"sanitized_by"`
}

// ──────────────────────────────────────────────────────────────
// ParsedSkill
// ──────────────────────────────────────────────────────────────

// ParsedSkill is the in-memory representation produced by ParseSKILLMD.
type ParsedSkill struct {
	Name         string
	Description  string
	Version      string
	Capabilities []core.Capability
	Commands     []*Command
	// Extra frontmatter fields (informational, not used at runtime)
	Authors  []Author
	Homepage string
	Requires Requirements
	Security SecurityInfo
}

// ──────────────────────────────────────────────────────────────
// Compiled regexes used by the tool-section parser
// ──────────────────────────────────────────────────────────────

var (
	// **Description:** some text
	reDesc = regexp.MustCompile(`(?m)^\*\*Description:\*\*\s*(.+)`)

	// **Command:** followed by a fenced code block (bash, sh, or no lang tag)
	reCmd = regexp.MustCompile("(?s)\\*\\*Command:\\*\\*\\s*```(?:bash|sh)?\\n(.+?)```")

	// **Timeout:** 30
	reTimeout = regexp.MustCompile(`(?m)^\*\*Timeout:\*\*\s*(\d+)`)

	// - `paramName` (type): description
	//   backtick = \x60
	reParam = regexp.MustCompile("(?m)^-\\s+`([^`]+)`\\s+\\(([^)]+)\\)(?::\\s*(.*))?")

	// **Validation:** followed by a fenced yaml code block
	reValidation = regexp.MustCompile("(?s)\\*\\*Validation:\\*\\*\\s*```yaml\\n(.+?)```")
)

// ──────────────────────────────────────────────────────────────
// Public API
// ──────────────────────────────────────────────────────────────

// LoadCLISkillsFromDirectory loads all .md files from dir as CLI skills.
func LoadCLISkillsFromDirectory(dir string, executor *CLIExecutor) ([]*CLISkill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var loaded []*CLISkill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		skill, err := LoadCLISkill(path, executor)
		if err != nil {
			logger.Log.Warn("failed to load CLI skill",
				"file", entry.Name(),
				"error", err)
			continue
		}

		loaded = append(loaded, skill)
	}

	logger.Log.Info("loaded CLI skills",
		"count", len(loaded),
		"directory", dir)

	return loaded, nil
}

// LoadCLISkill parses a single SKILL.md file and returns a CLISkill.
func LoadCLISkill(path string, executor *CLIExecutor) (*CLISkill, error) {
	logger.Log.Info("loading CLI skill", "path", path)

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	parsed, err := ParseSKILLMD(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse SKILL.md: %w", err)
	}

	// Security warnings on command templates
	for _, cmd := range parsed.Commands {
		if warnings := validateCommandTemplate(cmd.Template); len(warnings) > 0 {
			for _, w := range warnings {
				logger.Log.Warn("CLI skill security warning",
					"skill", parsed.Name,
					"tool", cmd.Name,
					"warning", w)
			}
		}
	}

	skill := NewCLISkill(parsed, executor)

	logger.Log.Info("loaded CLI skill",
		"name", parsed.Name,
		"version", parsed.Version,
		"tools", len(parsed.Commands),
		"capabilities", len(parsed.Capabilities))

	return skill, nil
}

// ParseSKILLMD parses a SKILL.md file that uses YAML frontmatter.
//
// Expected layout:
//
//	---
//	name: my-skill
//	description: …
//	version: 1.0.0
//	capabilities: [foo, bar]
//	…
//	---
//	# Heading (ignored at runtime)
//	…prose…
//	## Tools
//	### tool_name
//	**Description:** …
//	**Command:**
//	```bash
//	curl …
//	```
//	**Parameters:**
//	- `param` (string): description
//	**Timeout:** 10
//	---           ← separator between tools
//	### next_tool
//	…
func ParseSKILLMD(content string) (*ParsedSkill, error) {
	fm, body, err := extractFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("extract frontmatter: %w", err)
	}

	if fm.Name == "" {
		return nil, fmt.Errorf("frontmatter 'name' field is required")
	}

	version := fm.Version
	if version == "" {
		version = "1.0.0"
	}

	caps := make([]core.Capability, 0, len(fm.Capabilities))
	for _, c := range fm.Capabilities {
		if c != "" {
			caps = append(caps, core.Capability(c))
		}
	}

	commands, err := parseToolsSection(body)
	if err != nil {
		return nil, fmt.Errorf("parse tools: %w", err)
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("at least one ### tool section is required")
	}

	return &ParsedSkill{
		Name:         fm.Name,
		Description:  fm.Description,
		Version:      version,
		Capabilities: caps,
		Commands:     commands,
		Authors:      fm.Authors,
		Homepage:     fm.Homepage,
		Requires:     fm.Requires,
		Security:     fm.Security,
	}, nil
}

// ──────────────────────────────────────────────────────────────
// Internal helpers
// ──────────────────────────────────────────────────────────────

// extractFrontmatter splits "---\n<yaml>\n---\n<body>" into its two parts.
func extractFrontmatter(content string) (*Frontmatter, string, error) {
	if !strings.HasPrefix(content, "---") {
		return nil, "", fmt.Errorf("file must begin with YAML frontmatter (---)")
	}

	// Skip the opening ---
	rest := strings.TrimPrefix(content, "---")
	if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}

	// Find the closing ---
	closingIdx := strings.Index(rest, "\n---")
	if closingIdx == -1 {
		return nil, "", fmt.Errorf("frontmatter closing '---' not found")
	}

	yamlContent := rest[:closingIdx]
	body := rest[closingIdx+4:] // skip \n---

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, "", fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	return &fm, body, nil
}

// parseToolsSection finds "## Tools" in the body and parses every ### block.
func parseToolsSection(body string) ([]*Command, error) {
	toolsMarker := "## Tools"
	idx := strings.Index(body, toolsMarker)
	if idx == -1 {
		return nil, fmt.Errorf("'## Tools' section not found")
	}

	toolsBody := body[idx+len(toolsMarker):]

	// Split on lines that are exactly "---" – these separate tool definitions.
	sections := splitOnSeparator(toolsBody)

	var commands []*Command
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" || !strings.HasPrefix(section, "### ") {
			continue
		}

		cmd, err := parseToolSection(section)
		if err != nil {
			return nil, fmt.Errorf("parse tool section: %w", err)
		}
		commands = append(commands, cmd)
	}

	return commands, nil
}

// splitOnSeparator splits content on lines that are exactly "---".
func splitOnSeparator(content string) []string {
	var sections []string
	var cur strings.Builder

	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "---" {
			sections = append(sections, cur.String())
			cur.Reset()
		} else {
			cur.WriteString(line)
			cur.WriteByte('\n')
		}
	}
	if cur.Len() > 0 {
		sections = append(sections, cur.String())
	}
	return sections
}

// parseToolSection parses a single ### tool block into a Command.
func parseToolSection(section string) (*Command, error) {
	cmd := &Command{
		Timeout:   30,
		ParamDefs: []ParamDef{},
	}

	// First line: ### tool_name
	firstNL := strings.Index(section, "\n")
	if firstNL == -1 {
		firstNL = len(section)
	}
	cmd.Name = strings.TrimSpace(strings.TrimPrefix(section[:firstNL], "### "))
	if cmd.Name == "" {
		return nil, fmt.Errorf("tool section has empty name")
	}

	// Description
	if m := reDesc.FindStringSubmatch(section); m != nil {
		cmd.Description = strings.TrimSpace(m[1])
	}

	// Command template (inside fenced code block)
	if m := reCmd.FindStringSubmatch(section); m != nil {
		cmd.Template = strings.TrimSpace(m[1])
	}
	if cmd.Template == "" {
		return nil, fmt.Errorf("tool %q has no **Command:** block", cmd.Name)
	}

	// Timeout
	if m := reTimeout.FindStringSubmatch(section); m != nil {
		if t, err := strconv.Atoi(m[1]); err == nil {
			cmd.Timeout = t
		}
	}

	// Parameters
	for _, m := range reParam.FindAllStringSubmatch(section, -1) {
		desc := ""
		if len(m) > 3 {
			desc = strings.TrimSpace(m[3])
		}
		cmd.ParamDefs = append(cmd.ParamDefs, ParamDef{
			Name:        m[1],
			Type:        strings.ToLower(strings.TrimSpace(m[2])),
			Description: desc,
		})
	}

	// Derive Parameters []string from ParamDefs for backward compatibility
	cmd.Parameters = make([]string, 0, len(cmd.ParamDefs))
	for _, p := range cmd.ParamDefs {
		cmd.Parameters = append(cmd.Parameters, p.Name)
	}

	// Validation block
	if m := reValidation.FindStringSubmatch(section); m != nil {
		v, err := parseValidationYAML(m[1])
		if err != nil {
			logger.Log.Warn("failed to parse validation block",
				"tool", cmd.Name, "error", err)
		} else {
			cmd.Validation = v
		}
	}

	return cmd, nil
}

// validationYAML is the intermediate struct for the **Validation:** yaml block.
type validationYAML struct {
	AllowedCommands []string `yaml:"allowed_commands"`
	DeniedPatterns  []string `yaml:"denied_patterns"`
	MaxOutputSize   int      `yaml:"max_output_size"`
	RequireConfirm  bool     `yaml:"require_confirm"`
}

func parseValidationYAML(content string) (*ValidationRules, error) {
	var v validationYAML
	if err := yaml.Unmarshal([]byte(content), &v); err != nil {
		return nil, err
	}
	return &ValidationRules{
		AllowedCommands: v.AllowedCommands,
		DeniedPatterns:  v.DeniedPatterns,
		MaxOutputSize:   v.MaxOutputSize,
		RequireConfirm:  v.RequireConfirm,
	}, nil
}

// validateCommandTemplate checks for obviously dangerous patterns in a template.
func validateCommandTemplate(template string) []string {
	dangerousPatterns := map[string]string{
		`rm\s+-rf`:     "uses 'rm -rf' which can delete important files",
		`--force`:      "uses '--force' flag which can be dangerous",
		`sudo`:         "uses 'sudo' which requires elevated privileges",
		`/etc/`:        "accesses /etc/ directory",
		`curl.*\|.*sh`: "pipes curl output to shell",
		`wget.*\|.*sh`: "pipes wget output to shell",
		`>\s*/dev/`:    "writes to /dev/ devices",
	}

	var warnings []string
	lower := strings.ToLower(template)
	for pattern, message := range dangerousPatterns {
		prefix := strings.Split(pattern, `\`)[0]
		if strings.Contains(lower, strings.ToLower(prefix)) {
			warnings = append(warnings, message)
		}
	}
	return warnings
}
