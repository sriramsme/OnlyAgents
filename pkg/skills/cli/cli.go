package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// CLISkill represents a skill loaded from a SKILL.md file
type CLISkill struct {
	*skills.BaseSkill

	// From SKILL.md
	capabilities []core.Capability
	toolDefs     []tools.ToolDef
	commands     map[string]*Command // toolName -> command

	// Runtime
	executor *CLIExecutor
}

// Command represents a CLI command from SKILL.md
type Command struct {
	Name        string           // Tool name
	Description string           // What it does
	Template    string           // Command template with {{param}} placeholders
	Parameters  []string         // Required parameters
	Validation  *ValidationRules // Optional validation
	Timeout     int              // Seconds, default 30
}

// ValidationRules for command execution
type ValidationRules struct {
	AllowedCommands []string // Whitelist of allowed base commands
	DeniedPatterns  []string // Blacklist patterns (regex)
	MaxOutputSize   int      // Max bytes of output
	RequireConfirm  bool     // Require user confirmation
}

// NewCLISkill creates a CLI skill from parsed definition
func NewCLISkill(definition *ParsedSkill, executor *CLIExecutor) *CLISkill {
	// Create base skill
	base := skills.NewBaseSkill(
		definition.Name,
		definition.Description,
		definition.Version,
		skills.SkillTypeCLI,
	)

	// Build tools from commands
	toolDefs := make([]tools.ToolDef, 0, len(definition.Commands))
	commandMap := make(map[string]*Command)

	for _, cmd := range definition.Commands {
		// Build tool parameters
		params := tools.BuildParams(
			buildParamProps(cmd.Parameters),
			cmd.Parameters,
		)

		toolDef := tools.NewToolDef(
			cmd.Name,
			cmd.Description,
			params,
		)

		toolDefs = append(toolDefs, toolDef)
		commandMap[cmd.Name] = cmd
	}

	return &CLISkill{
		BaseSkill:    base,
		capabilities: definition.Capabilities,
		toolDefs:     toolDefs,
		commands:     commandMap,
		executor:     executor,
	}
}

// Initialize implements Skill interface
func (s *CLISkill) Initialize(deps skills.SkillDeps) error {
	s.SetOutbox(deps.Outbox)
	return nil
}

// Shutdown implements Skill interface
func (s *CLISkill) Shutdown() error {
	return nil
}

// RequiredCapabilities implements Skill interface
func (s *CLISkill) RequiredCapabilities() []core.Capability {
	return s.capabilities
}

// Tools implements Skill interface
func (s *CLISkill) Tools() []tools.ToolDef {
	return s.toolDefs
}

// Execute implements Skill interface
func (s *CLISkill) Execute(ctx context.Context, toolName string, args []byte) (any, error) {
	// Parse JSON args
	var params map[string]any
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	// Get command definition
	cmd, ok := s.commands[toolName]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	// Build command from template
	command := s.buildCommand(cmd.Template, params)

	// Validate command
	if err := s.validateCommand(command, cmd.Validation); err != nil {
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	// Execute via executor
	result, err := s.executor.Execute(ctx, command, cmd.Timeout)
	if err != nil {
		return nil, fmt.Errorf("command execution failed: %w", err)
	}

	return map[string]any{
		"output":      result.Stdout,
		"error":       result.Stderr,
		"exit_code":   result.ExitCode,
		"duration_ms": result.Duration.Milliseconds(),
	}, nil
}

// buildCommand replaces {{param}} placeholders with actual values
func (s *CLISkill) buildCommand(template string, params map[string]any) string {
	command := template

	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	command = re.ReplaceAllStringFunc(command, func(match string) string {
		// Extract parameter name
		paramName := strings.Trim(match, "{}")

		if val, ok := params[paramName]; ok {
			// TODO: Proper shell escaping here!
			return fmt.Sprintf("%v", val)
		}

		return match // Keep placeholder if param not found
	})

	return command
}

// validateCommand validates command before execution
func (s *CLISkill) validateCommand(command string, rules *ValidationRules) error {
	if rules == nil {
		return nil
	}

	// Check allowed commands whitelist
	if len(rules.AllowedCommands) > 0 {
		allowed := false
		for _, allowedCmd := range rules.AllowedCommands {
			if strings.HasPrefix(command, allowedCmd) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("command not in whitelist: %s", command)
		}
	}

	// Check denied patterns blacklist
	for _, pattern := range rules.DeniedPatterns {
		matched, err := regexp.MatchString(pattern, command)
		if err != nil {
			return fmt.Errorf("invalid pattern: %w", err)
		}
		if matched {
			return fmt.Errorf("command matches denied pattern: %s", pattern)
		}
	}

	// Check for dangerous commands
	dangerousPatterns := []string{
		`rm\s+-rf`,
		`dd\s+if=`,
		`mkfs`,
		`:\(\)\{.*\}`,  // Fork bomb
		`curl.*\|.*sh`, // Piping to shell
		`wget.*\|.*sh`,
	}

	for _, pattern := range dangerousPatterns {
		matched, err := regexp.MatchString(pattern, command)
		if err == nil && matched {
			return fmt.Errorf("dangerous command detected: %s", command)
		}
	}

	return nil
}

// buildParamProps builds tool parameter properties with type inference
func buildParamProps(params []string) map[string]tools.Property {
	props := make(map[string]tools.Property)
	for _, param := range params {
		props[param] = inferParamType(param)
	}
	return props
}

// inferParamType infers parameter type from name
func inferParamType(paramName string) tools.Property {
	lower := strings.ToLower(paramName)

	// Integers
	if strings.Contains(lower, "count") ||
		strings.Contains(lower, "limit") ||
		strings.Contains(lower, "port") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "size") {
		return tools.Property{
			Type:        "integer",
			Description: fmt.Sprintf("Parameter: %s", paramName),
		}
	}

	// Booleans
	if strings.HasPrefix(lower, "is_") ||
		strings.HasPrefix(lower, "enable_") ||
		strings.HasPrefix(lower, "disable_") ||
		strings.Contains(lower, "flag") {
		return tools.Property{
			Type:        "boolean",
			Description: fmt.Sprintf("Parameter: %s", paramName),
		}
	}

	// Arrays (very basic heuristic)
	if strings.HasSuffix(paramName, "s") &&
		!strings.HasSuffix(lower, "status") &&
		!strings.HasSuffix(lower, "class") {
		return tools.Property{
			Type:        "array",
			Items:       &tools.Property{Type: "string"},
			Description: fmt.Sprintf("Parameter: %s", paramName),
		}
	}

	// Default: string
	return tools.Property{
		Type:        "string",
		Description: fmt.Sprintf("Parameter: %s", paramName),
	}
}
