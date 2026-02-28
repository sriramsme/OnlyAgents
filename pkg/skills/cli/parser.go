package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// ParsedSkill represents parsed SKILL.md content
type ParsedSkill struct {
	Name         string
	Description  string
	Version      string
	Capabilities []core.Capability
	Commands     []*Command
}

// LoadCLISkillsFromDirectory loads all SKILL.md files from a directory
func LoadCLISkillsFromDirectory(dir string, executor *CLIExecutor) ([]*CLISkill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var skills []*CLISkill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		skill, err := LoadCLISkill(path, executor)
		if err != nil {
			// Log error but continue loading other skills
			logger.Log.Warn("failed to load CLI skill",
				"file", entry.Name(),
				"error", err)
			continue
		}

		skills = append(skills, skill)
	}

	logger.Log.Info("loaded CLI skills",
		"count", len(skills),
		"directory", dir)

	return skills, nil
}

// LoadCLISkill loads a single SKILL.md file
func LoadCLISkill(path string, executor *CLIExecutor) (*CLISkill, error) {
	logger.Log.Info("loading CLI skill", "path", path)

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	content := string(data)

	// Parse SKILL.md format
	parsed, err := ParseSKILLMD(content)
	if err != nil {
		return nil, fmt.Errorf("parse SKILL.md: %w", err)
	}

	// Security validation with warnings
	for _, cmd := range parsed.Commands {
		if warnings := validateCommandTemplate(cmd.Template); len(warnings) > 0 {
			for _, warning := range warnings {
				logger.Log.Warn("CLI skill security warning",
					"skill", parsed.Name,
					"tool", cmd.Name,
					"warning", warning)
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

// ParseSKILLMD parses SKILL.md content
// nolint:gocyclo
func ParseSKILLMD(content string) (*ParsedSkill, error) {
	lines := strings.Split(content, "\n")

	var (
		name         string
		description  string
		version      = "1.0.0"
		capabilities []core.Capability
		commands     []*Command

		inMetadata  bool
		inTool      bool
		currentTool *Command
	)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse metadata section
		if strings.HasPrefix(line, "# ") {
			name = strings.TrimPrefix(line, "# ")
			inMetadata = true
			continue
		}

		if inMetadata {
			if strings.HasPrefix(line, "Description:") {
				description = strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
			}
			if strings.HasPrefix(line, "Version:") {
				version = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
			}
			if strings.HasPrefix(line, "Capabilities:") {
				capsLine := strings.TrimPrefix(line, "Capabilities:")
				capStrs := strings.Split(capsLine, ",")
				for _, capStr := range capStrs {
					capStr = strings.TrimSpace(capStr)
					if capStr != "" {
						capabilities = append(capabilities, core.Capability(capStr))
					}
				}
			}

			if strings.HasPrefix(line, "## Tools") {
				inMetadata = false
			}
			continue
		}

		// Parse tool definitions
		if strings.HasPrefix(line, "### ") {
			if currentTool != nil {
				commands = append(commands, currentTool)
			}

			toolName := strings.TrimPrefix(line, "### ")
			currentTool = &Command{
				Name:       toolName,
				Parameters: []string{},
				Timeout:    30,
			}
			inTool = true
			continue
		}

		if inTool && currentTool != nil {
			if strings.HasPrefix(line, "Description:") {
				currentTool.Description = strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
			}
			if strings.HasPrefix(line, "Command:") {
				currentTool.Template = strings.TrimSpace(strings.TrimPrefix(line, "Command:"))
			}
			if strings.HasPrefix(line, "Parameters:") {
				paramLine := strings.TrimPrefix(line, "Parameters:")
				params := strings.Split(paramLine, ",")
				for _, p := range params {
					p = strings.TrimSpace(p)
					if p != "" {
						currentTool.Parameters = append(currentTool.Parameters, p)
					}
				}
			}
			if strings.HasPrefix(line, "Timeout:") {
				_, err := fmt.Sscanf(line, "Timeout: %d", &currentTool.Timeout) //nolint:errcheck
				if err != nil {
					fmt.Printf("parse timeout: %s", err)
				}

			}
		}
	}

	// Add last tool
	if currentTool != nil {
		commands = append(commands, currentTool)
	}

	// Validation
	if name == "" {
		return nil, fmt.Errorf("skill name is required (must have # heading)")
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("at least one tool is required (must have ### tool sections)")
	}

	return &ParsedSkill{
		Name:         name,
		Description:  description,
		Version:      version,
		Capabilities: capabilities,
		Commands:     commands,
	}, nil
}

// validateCommandTemplate checks for security issues in command templates
func validateCommandTemplate(template string) []string {
	var warnings []string

	dangerousPatterns := map[string]string{
		`rm\s+-rf`:     "uses 'rm -rf' which can delete important files",
		`--force`:      "uses '--force' flag which can be dangerous",
		`sudo`:         "uses 'sudo' which requires elevated privileges",
		`/etc/`:        "accesses /etc/ directory",
		`curl.*\|.*sh`: "pipes curl output to shell",
		`wget.*\|.*sh`: "pipes wget output to shell",
		`>\s*/dev/`:    "writes to /dev/ devices",
	}

	for pattern, message := range dangerousPatterns {
		// Basic contains check (not full regex for performance)
		if strings.Contains(strings.ToLower(template), strings.ToLower(strings.Split(pattern, `\`)[0])) {
			warnings = append(warnings, message)
		}
	}

	return warnings
}
