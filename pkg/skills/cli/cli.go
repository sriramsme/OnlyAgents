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

// CLISkill executes structured commands defined in YAML skill files.
// The LLM supplies parameter values only — the binary and arg templates
// are always config-defined, never user-supplied.
type CLISkill struct {
	*skills.BaseSkill

	ctx      context.Context
	cancel   context.CancelFunc
	commands map[string]*skills.ToolEntry

	executor *CLIExecutor
}

// ── Constructor

func NewCLISkill(ctx context.Context, cfg skills.Config, conn connectors.Connector) (skills.Skill, error) {
	if conn != nil {
		return nil, fmt.Errorf("cli skill does not accept a connector")
	}

	executor := NewCLIExecutor(&cfg.Executor, cfg.Security, cfg.Requires.Env)

	missing, err := checkRequiredBins(cfg.Requires)
	if err != nil {
		return nil, fmt.Errorf("skill %s: check required bins: %w", cfg.Name, err)
	}
	if len(missing) > 0 {
		switch cfg.Executor.MissingBinBehavior {
		case "disable":
			return nil, fmt.Errorf("skill %s disabled: missing bins: %s", cfg.Name, strings.Join(missing, ", "))
		case "warn":
			logger.Log.Warn("skill missing required bins — some tools may fail",
				"skill", cfg.Name,
				"missing", missing)
		default: // "error"
			return nil, fmt.Errorf("skill %s: required bins not found: %s", cfg.Name, strings.Join(missing, ", "))
		}
	}

	toolDefs := make([]tools.ToolDef, 0, len(cfg.Tools))
	commandMap := make(map[string]*skills.ToolEntry, len(cfg.Tools))

	for i := range cfg.Tools {
		t := &cfg.Tools[i]

		if t.Exec.Command == "" {
			logger.Log.Warn("tool has no exec.command, skipping",
				"skill", cfg.Name, "tool", t.Name)
			continue
		}
		if t.Timeout == 0 {
			t.Timeout = 30
		}
		if t.Access == "" {
			t.Access = "read"
		}
		if !accessPermits(cfg.AccessLevel, t.Access) {
			continue
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

	logger.Log.Info("initialized cli skill",
		"skill", cfg.Name,
		"tools", len(toolDefs),
		"access_level", cfg.AccessLevel)

	groupDefs := make(map[tools.ToolGroup]string, len(cfg.Groups))
	for name, desc := range cfg.Groups {
		groupDefs[tools.ToolGroup(name)] = desc
	}

	cliCtx, cancel := context.WithCancel(ctx)
	base := skills.NewBaseSkillFromConfig(cfg, skills.SkillTypeCLI, toolDefs, groupDefs)

	return &CLISkill{
		BaseSkill: base,
		commands:  commandMap,
		executor:  executor,
		ctx:       cliCtx,
		cancel:    cancel,
	}, nil
}

// ── Skill interface

func (s *CLISkill) Initialize() error { return nil }

func (s *CLISkill) Shutdown() error {
	s.cancel()
	return nil
}

func (s *CLISkill) Execute(ctx context.Context, toolName string, rawArgs []byte) tools.ToolExecution {
	var params map[string]any
	if err := json.Unmarshal(rawArgs, &params); err != nil {
		return tools.ExecErr(fmt.Errorf("parse args: %w", err))
	}

	entry, ok := s.commands[toolName]
	if !ok {
		return tools.ExecErr(fmt.Errorf("tool not found: %s", toolName))
	}

	// Binary is always from config — never from LLM input.
	binary := entry.Exec.Command
	args := buildArgs(entry.Exec.Args, params)

	// StdinParam: some tools (e.g. fs_write via tee) pipe a param as stdin
	// rather than passing it as an arg, avoiding shell quoting entirely.
	stdin := ""
	if entry.Exec.StdinParam != "" {
		if v, ok := params[entry.Exec.StdinParam]; ok {
			stdin = fmt.Sprintf("%v", v)
		}
	}

	callID := uuid.NewString()
	outputDir := filepath.Join(s.executor.security.WorkingDir, "output", callID)
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return tools.ExecErr(fmt.Errorf("create output dir: %w", err))
	}

	result, err := s.executor.Execute(ctx, binary, args, stdin, entry.Timeout, outputDir)
	if err != nil {
		return tools.ExecErr(fmt.Errorf("execution failed: %w", err))
	}

	producedFiles, err := scanOutputDir(outputDir)
	if err != nil {
		logger.Log.Warn("failed to scan output dir",
			"output_dir", outputDir, "err", err)
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

// ── Helpers

// buildArgs interpolates {{param}} placeholders in arg templates.
// No shell escaping is needed — args go directly to exec.Command, not a shell.
func buildArgs(templates []string, params map[string]any) []string {
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	out := make([]string, len(templates))
	for i, tmpl := range templates {
		out[i] = re.ReplaceAllStringFunc(tmpl, func(match string) string {
			name := strings.Trim(match, "{}")
			if val, ok := params[name]; ok {
				return fmt.Sprintf("%v", val)
			}
			return match // leave unresolved placeholder as-is
		})
	}
	return out
}

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

func scanOutputDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
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
