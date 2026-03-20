package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"

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
	commands map[string]*skills.ToolEntry // toolName → command

	executor *CLIExecutor
}

// ──────────────────────────────────────────────────────────────
// Constructor
// ──────────────────────────────────────────────────────────────

// NewCLISkill creates a CLISkill from a ParsedSkill definition.
func NewCLISkill(ctx context.Context, cfg skills.Config, conn connectors.Connector,
) (skills.Skill, error) {
	// cli doesnt need a connector, check if its empty
	if conn != nil {
		return nil, fmt.Errorf("cli skill: connector is not empty")
	}
	executor := NewCLIExecutor(ctx, &cfg.Executor, cfg.Security, cfg.Requires.Env)

	toolDefs := make([]tools.ToolDef, 0, len(cfg.Tools))
	commandMap := make(map[string]*skills.ToolEntry, len(cfg.Tools))

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
			tools.ToolGroup(t.Group),
		))
		commandMap[t.Name] = t
	}

	cliCtx, cancel := context.WithCancel(ctx)
	logger.Log.Info("parsed cli skill",
		"skill", cfg.Name,
		"tools", len(toolDefs),
		"commands", len(commandMap))

	groupDefs := make(map[tools.ToolGroup]string)
	for name, desc := range cfg.Groups {
		groupDefs[tools.ToolGroup(name)] = desc
	}
	base := skills.NewBaseSkillFromConfig(cfg, skills.SkillTypeCLI, toolDefs, groupDefs)
	return &CLISkill{
		BaseSkill: base,
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

// Execute runs the CLI command corresponding to toolName.
func (s *CLISkill) Execute(ctx context.Context, toolName string, args []byte) tools.ToolExecution {
	var params map[string]any
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ExecErr(fmt.Errorf("parse args: %w", err))
	}

	cmd, ok := s.commands[toolName]
	if !ok {
		return tools.ExecErr(fmt.Errorf("tool not found: %s", toolName))
	}

	command := buildCommand(cmd.Command, params)
	if err := validateCommand(command, cmd.Validation); err != nil {
		return tools.ExecErr(fmt.Errorf("command validation failed: %w", err))
	}

	// Create a per-call output directory. CLI commands write files here via
	// the OUTPUT_DIR env var. We scan it after execution to collect produced files.
	callID := uuid.NewString()
	outputDir := filepath.Join(s.executor.security.WorkingDir, "output", callID)
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return tools.ExecErr(fmt.Errorf("create output dir: %w", err))
	}

	result, err := s.executor.Execute(ctx, command, cmd.Timeout, outputDir)
	if err != nil {
		return tools.ExecErr(fmt.Errorf("command execution failed: %w", err))
	}

	// Collect any files the command wrote to outputDir.
	producedFiles, err := scanOutputDir(outputDir)
	if err != nil {
		// Non-fatal — command succeeded, we just couldn't collect files.
		logger.Log.Warn("failed to scan output dir",
			"output_dir", outputDir,
			"err", err)
	}

	return tools.ToolExecution{
		Result: map[string]any{
			"output":      result.Stdout,
			"error":       result.Stderr,
			"exit_code":   result.ExitCode,
			"duration_ms": result.Duration.Milliseconds(),
		},
		ProducedFiles: producedFiles,
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────
func checkRequiredBins(requires skills.Requirements) (missing []string, err error) {
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

// scanOutputDir returns absolute paths of all files written to dir.
// Subdirectories are ignored — commands should write flat files to OUTPUT_DIR.
func scanOutputDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // command didn't write anything, that's fine
		}
		return nil, fmt.Errorf("read output dir: %w", err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	return paths, nil
}

// validateCommand checks against skill-defined rules and hardcoded dangerous patterns.
func validateCommand(command string, rules *skills.Validation) error {
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

func buildParamProps(defs []skills.ParamDef) map[string]tools.Property {
	props := make(map[string]tools.Property, len(defs))
	for _, d := range defs {
		props[d.Name] = paramDefToProperty(d)
	}
	return props
}

func paramDefToProperty(d skills.ParamDef) tools.Property {
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
