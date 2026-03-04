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

// CLISkill represents a skill loaded from a SKILL.md file.
type CLISkill struct {
	*skills.BaseSkill

	capabilities []core.Capability
	toolDefs     []tools.ToolDef
	commands     map[string]*Command // toolName → command

	executor *CLIExecutor
}

// Command represents a CLI command parsed from a SKILL.md tool section.
type Command struct {
	Name        string           // Tool name  (### heading)
	Description string           // **Description:**
	Template    string           // **Command:** (bash code block)
	ParamDefs   []ParamDef       // **Parameters:** bullet list (authoritative)
	Parameters  []string         // Derived from ParamDefs for backward-compat
	Validation  *ValidationRules // **Validation:** yaml block (optional)
	Timeout     int              // **Timeout:** (seconds, default 30)
}

// ParamDef holds the name, explicit type, and description of a single parameter.
type ParamDef struct {
	Name        string // e.g. "location"
	Type        string // e.g. "string", "number", "integer", "boolean", "array"
	Description string // e.g. "City name, airport code, or coordinates"
}

// ValidationRules for command execution.
type ValidationRules struct {
	AllowedCommands []string // Whitelist of allowed base commands
	DeniedPatterns  []string // Blacklist patterns (regex)
	MaxOutputSize   int      // Max bytes of output
	RequireConfirm  bool     // Require user confirmation
}

// ──────────────────────────────────────────────────────────────
// Constructor
// ──────────────────────────────────────────────────────────────

// NewCLISkill creates a CLISkill from a ParsedSkill definition.
func NewCLISkill(definition *ParsedSkill, executor *CLIExecutor) *CLISkill {
	name := tools.SkillName(definition.Name)
	base := skills.NewBaseSkill(
		name,
		definition.Description,
		definition.Version,
		skills.SkillTypeCLI,
	)

	toolDefs := make([]tools.ToolDef, 0, len(definition.Commands))
	commandMap := make(map[string]*Command, len(definition.Commands))

	for _, cmd := range definition.Commands {
		props := buildParamProps(cmd.ParamDefs)
		params := tools.BuildParams(props, cmd.Parameters)
		toolDefs = append(toolDefs, tools.NewToolDef(
			tools.SkillName(definition.Name), // dynamic, runtime cast
			cmd.Name,
			cmd.Description,
			params,
		))
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

// ──────────────────────────────────────────────────────────────
// Skill interface
// ──────────────────────────────────────────────────────────────

func (s *CLISkill) Initialize(deps skills.SkillDeps) error {
	s.SetOutbox(deps.Outbox)
	return nil
}

func (s *CLISkill) Shutdown() error { return nil }

func (s *CLISkill) RequiredCapabilities() []core.Capability { return s.capabilities }

func (s *CLISkill) Tools() []tools.ToolDef { return s.toolDefs }

// Execute runs the CLI command corresponding to toolName.
func (s *CLISkill) Execute(ctx context.Context, toolName string, args []byte) (any, error) {
	var params map[string]any
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	cmd, ok := s.commands[toolName]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	command := s.buildCommand(cmd.Template, params)

	if err := s.validateCommand(command, cmd.Validation); err != nil {
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

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

// ──────────────────────────────────────────────────────────────
// Internal helpers
// ──────────────────────────────────────────────────────────────

// buildCommand replaces {{param}} placeholders with actual values.
func (s *CLISkill) buildCommand(template string, params map[string]any) string {
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	return re.ReplaceAllStringFunc(template, func(match string) string {
		paramName := strings.Trim(match, "{}")
		if val, ok := params[paramName]; ok {
			// TODO: proper shell escaping
			return fmt.Sprintf("%v", val)
		}
		return match // leave placeholder if param missing
	})
}

// validateCommand checks the command against the supplied rules and a hardcoded
// set of dangerous patterns.
func (s *CLISkill) validateCommand(command string, rules *ValidationRules) error {
	if rules != nil {
		// Allowed-commands whitelist
		if len(rules.AllowedCommands) > 0 {
			allowed := false
			for _, a := range rules.AllowedCommands {
				if strings.HasPrefix(command, a) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("command not in whitelist: %s", command)
			}
		}

		// Denied patterns from the skill definition
		for _, pattern := range rules.DeniedPatterns {
			matched, err := regexp.MatchString(pattern, command)
			if err != nil {
				return fmt.Errorf("invalid denied pattern %q: %w", pattern, err)
			}
			if matched {
				return fmt.Errorf("command matches denied pattern: %s", pattern)
			}
		}
	}

	// Hard-coded dangerous patterns (always enforced)
	dangerousPatterns := []string{
		`rm\s+-rf`,
		`dd\s+if=`,
		`mkfs`,
		`:\(\)\{.*\}`,  // fork bomb
		`curl.*\|.*sh`, // pipe to shell
		`wget.*\|.*sh`,
	}
	for _, pattern := range dangerousPatterns {
		matched, err := regexp.MatchString(pattern, command)
		if err == nil && matched {
			return fmt.Errorf("dangerous command pattern detected: %s", command)
		}
	}

	return nil
}

// buildParamProps converts a []ParamDef into the property map expected by
// tools.BuildParams. Types declared in the skill file are used directly;
// unknown types fall back to "string".
func buildParamProps(defs []ParamDef) map[string]tools.Property {
	props := make(map[string]tools.Property, len(defs))
	for _, d := range defs {
		props[d.Name] = paramDefToProperty(d)
	}
	return props
}

// paramDefToProperty converts a single ParamDef to a tools.Property.
// The Type field uses the explicit type from the SKILL.md file, mapping
// JSON-Schema / OpenAPI style names to the tools package constants.
func paramDefToProperty(d ParamDef) tools.Property {
	description := d.Description
	if description == "" {
		description = fmt.Sprintf("Parameter: %s", d.Name)
	}

	switch d.Type {
	case "integer", "int":
		return tools.Property{Type: "integer", Description: description}

	case "number", "float", "double":
		return tools.Property{Type: "number", Description: description}

	case "boolean", "bool":
		return tools.Property{Type: "boolean", Description: description}

	case "array":
		return tools.Property{
			Type:        "array",
			Items:       &tools.Property{Type: "string"},
			Description: description,
		}

	case "string", "":
		return tools.Property{Type: "string", Description: description}

	default:
		// Unknown type – default to string and log nothing; caller warned at load time.
		return tools.Property{Type: "string", Description: description}
	}
}
