package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func init() {
	skills.Register("cli", NewCLISkill)
}

// CLISkill represents a skill loaded from a SKILL.md file.
type CLISkill struct {
	*skills.BaseSkill

	ctx      context.Context
	cancel   context.CancelFunc
	toolDefs []tools.ToolDef
	commands map[string]*config.SkillToolEntry // toolName → command

	executor *CLIExecutor
}

// ──────────────────────────────────────────────────────────────
// Constructor
// ──────────────────────────────────────────────────────────────

// NewCLISkill creates a CLISkill from a ParsedSkill definition.
func NewCLISkill(ctx context.Context, cfg config.Skill, conn connectors.Connector,
	security config.SecurityConfig,
) (skills.Skill, error) {
	// cli doesnt need a connector, check if its empty
	if conn != nil {
		return nil, fmt.Errorf("cli skill: connector is not empty")
	}
	base := skills.NewBaseSkillFromConfig(cfg, skills.SkillTypeCLI)
	executor := NewCLIExecutor(ctx, &cfg.Executor, security, cfg.Requires.Env)

	toolDefs := make([]tools.ToolDef, 0, len(cfg.Tools))
	commandMap := make(map[string]*config.SkillToolEntry, len(cfg.Tools))

	logger.Log.Info("initializing cli skill",
		"skill", cfg.Name,
		"tools", len(cfg.Tools),
		"access_level", cfg.AccessLevel)

	missing, err := checkRequiredBins(cfg.Requires)
	if err != nil {
		return nil, fmt.Errorf("skill %s: check required bins: %w", cfg.Name, err)
	}
	if len(missing) > 0 {
		switch cfg.Executor.MissingBinBehavior {
		case "disable":
			return nil, fmt.Errorf("skill %s disabled: missing bins: %s", cfg.Name, strings.Join(missing, ", "))
		case "warn":
			logger.Log.Warn("skill missing required bins — tools may fail",
				"skill", cfg.Name,
				"missing", missing)
			// continue loading
		default: // "error" — default
			return nil, fmt.Errorf("skill %s: required bins not found: %s", cfg.Name, strings.Join(missing, ", "))
		}
	}

	for i := range cfg.Tools {
		t := &cfg.Tools[i] // pointer into slice — avoid copy

		if t.Timeout == 0 {
			t.Timeout = 30
		}
		if t.Access == "" {
			t.Access = "read"
		}

		if !accessPermits(cfg.AccessLevel, t.Access) {
			continue // tool exceeds skill access level — skip
		}

		params := make([]string, len(t.Parameters))
		for i, p := range t.Parameters {
			params[i] = p.Name
		}

		toolDefs = append(toolDefs, tools.NewToolDef(
			cfg.Name,
			t.Name,
			t.Description,
			tools.BuildParams(buildParamProps(t.Parameters), params),
		))
		commandMap[t.Name] = t
	}

	cliCtx, cancel := context.WithCancel(ctx)
	logger.Log.Info("parsed cli skill",
		"skill", cfg.Name,
		"tools", len(toolDefs),
		"commands", len(commandMap))

	return &CLISkill{
		BaseSkill: base,
		toolDefs:  toolDefs,
		commands:  commandMap,
		executor:  executor,
		ctx:       cliCtx,
		cancel:    cancel,
	}, nil
}

// ── Skill interface ───────────────────────────────────────────────────────────

func (s *CLISkill) Initialize() error {
	return nil
}

func (s *CLISkill) Shutdown() error {
	s.cancel()
	return nil
}

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

	command := buildCommand(cmd.Command, params)

	if err := validateCommand(command, cmd.Validation); err != nil {
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

// ── Helpers ───────────────────────────────────────────────────────────────────
func checkRequiredBins(requires config.SkillRequirements) (missing []string, err error) {
	for _, bin := range requires.Bins {
		if _, err := exec.LookPath(bin.Name); err != nil {
			missing = append(missing, bin.Name)
		}
	}
	return missing, nil
}

func accessPermits(granted, required string) bool {
	levels := map[string]int{"read": 1, "write": 2, "admin": 3}
	return levels[granted] >= levels[required]
}

// buildCommand replaces {{param}} placeholders with actual values.
func buildCommand(template string, params map[string]any) string {
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	return re.ReplaceAllStringFunc(template, func(match string) string {
		name := strings.Trim(match, "{}")
		if val, ok := params[name]; ok {
			return fmt.Sprintf("%v", val) // TODO: proper shell escaping
		}
		return match
	})
}

// validateCommand checks against skill-defined rules and hardcoded dangerous patterns.
func validateCommand(command string, rules *config.SkillValidation) error {
	if rules != nil {
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

	for _, pattern := range []string{
		`rm\s+-rf`,
		`dd\s+if=`,
		`mkfs`,
		`:\(\)\{.*\}`,  // fork bomb
		`curl.*\|.*sh`, // pipe to shell
		`wget.*\|.*sh`,
	} {
		if matched, err := regexp.MatchString(pattern, command); err == nil && matched {
			return fmt.Errorf("dangerous command pattern detected: %s", command)
		}
	}

	return nil
}

func buildParamProps(defs []config.SkillParamDef) map[string]tools.Property {
	props := make(map[string]tools.Property, len(defs))
	for _, d := range defs {
		props[d.Name] = paramDefToProperty(d)
	}
	return props
}

func paramDefToProperty(d config.SkillParamDef) tools.Property {
	desc := d.Description
	if desc == "" {
		desc = fmt.Sprintf("Parameter: %s", d.Name)
	}
	switch d.Type {
	case "integer", "int":
		return tools.Property{Type: "integer", Description: desc}
	case "number", "float", "double":
		return tools.Property{Type: "number", Description: desc}
	case "boolean", "bool":
		return tools.Property{Type: "boolean", Description: desc}
	case "array":
		return tools.Property{Type: "array", Items: &tools.Property{Type: "string"}, Description: desc}
	default:
		return tools.Property{Type: "string", Description: desc}
	}
}
