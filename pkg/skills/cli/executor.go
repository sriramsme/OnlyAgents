// pkg/skills/cli/executor.go
package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// CLIExecutor executes shell commands securely
// This is NOT a connector - it's a command execution engine
type CLIExecutor struct {
	config      *config.ExecutorConfig
	ctx         context.Context
	requiredEnv []string
	cancel      context.CancelFunc
	security    config.SecurityConfig
}

// ExecutionResult holds command execution result
type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// NewCLIExecutor creates a CLI executor
func NewCLIExecutor(ctx context.Context, cfg *config.ExecutorConfig,
	security config.SecurityConfig, requiredEnv []string,
) *CLIExecutor {
	execCtx, cancel := context.WithCancel(ctx)

	return &CLIExecutor{
		config:      cfg,
		ctx:         execCtx,
		cancel:      cancel,
		security:    security,
		requiredEnv: requiredEnv,
	}
}

// Execute executes a shell command
func (e *CLIExecutor) Execute(ctx context.Context, command string, timeoutSec int) (*ExecutionResult, error) {
	if err := e.validateWorkingDir(); err != nil {
		return nil, err
	}

	if timeoutSec == 0 {
		timeoutSec = e.config.MaxExecutionTime
	}
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	if err := e.validateCommand(command); err != nil {
		logger.Log.Error("command validation failed", "command", command, "error", err)
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Ensure workspace dirs exist
	if err := os.MkdirAll(filepath.Join(e.security.WorkingDir, "tmp"), 0o700); err != nil {
		return nil, fmt.Errorf("create tmp dir: %w", err)
	}

	shell := e.config.AllowedShells[0]
	cmd := exec.CommandContext(execCtx, shell, "-c", command) //nolint:gosec
	cmd.Dir = e.security.WorkingDir
	cmd.Env = e.buildEnv(command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	logger.Log.Info("executing CLI command",
		"command", command,
		"timeout_sec", timeoutSec,
		"mode", e.security.ExecutionMode,
		"working_dir", e.security.WorkingDir)

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("execution error: %w", err)
		}
	}

	stdoutStr := sanitizeOutput(truncate(stdout.String(), e.config.MaxOutputSize))
	stderrStr := sanitizeOutput(truncate(stderr.String(), e.config.MaxOutputSize))

	if exitCode == 0 {
		logger.Log.Info("CLI command succeeded",
			"duration_ms", duration.Milliseconds(),
			"stdout_bytes", len(stdoutStr))
	} else {
		logger.Log.Warn("CLI command failed",
			"exit_code", exitCode,
			"duration_ms", duration.Milliseconds(),
			"stderr", stderrStr)
	}

	return &ExecutionResult{
		Stdout:   stdoutStr,
		Stderr:   stderrStr,
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}

func truncate(s string, max int) string {
	if max > 0 && len(s) > max {
		return s[:max] + "\n[OUTPUT TRUNCATED]"
	}
	return s
}

// Shutdown stops the executor
func (e *CLIExecutor) Shutdown() error {
	e.cancel()
	return nil
}

// validateCommand performs basic security validation
func (e *CLIExecutor) validateCommand(command string) error {
	// Check for empty command
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("empty command")
	}

	// Block extremely dangerous commands
	blocked := []string{
		"rm -rf /",
		"rm -rf /*",
		"dd if=/dev/zero",
		"mkfs",
		":(){ :|:& };:", // Fork bomb
	}

	cmdLower := strings.ToLower(command)
	for _, danger := range blocked {
		if strings.Contains(cmdLower, danger) {
			return fmt.Errorf("dangerous command blocked: contains '%s'", danger)
		}
	}

	return nil
}

func (e *CLIExecutor) validateWorkingDir() error {
	abs, err := filepath.Abs(e.security.WorkingDir)
	if err != nil {
		return fmt.Errorf("invalid working dir: %w", err)
	}
	// Ensure it resolves to somewhere under ~/.onlyagents
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	allowed := filepath.Join(home, ".onlyagents")
	if !strings.HasPrefix(abs, allowed) {
		return fmt.Errorf("working dir %q is outside allowed path %q", abs, allowed)
	}
	return nil
}

func (e *CLIExecutor) buildEnv(command string) []string {
	if e.security.ExecutionMode == "native" {
		return os.Environ() // native mode — full env
	}

	// restricted mode — minimal env, no secrets leak
	base := []string{
		"HOME=" + e.security.WorkingDir,
		"TMPDIR=" + filepath.Join(e.security.WorkingDir, "tmp"),
		"PATH=" + e.buildPath(),
		"TERM=dumb",
		"LANG=en_US.UTF-8",
	}

	// Only pass through env vars the skill explicitly declared it needs
	for _, envVar := range e.requiredEnv {
		if val := os.Getenv(envVar); val != "" {
			base = append(base, envVar+"="+val)
		}
	}

	return base
}

func (e *CLIExecutor) buildPath() string {
	if !e.security.AllowSystemBins {
		// only workspace bin dir
		return filepath.Join(e.security.WorkingDir, "bin")
	}
	return filepath.Join(e.security.WorkingDir, "bin") +
		":/usr/local/bin:/usr/bin:/bin"
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password)\s*[:=]\s*\S+`),
	regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`), // base64 secrets
}

func sanitizeOutput(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}
