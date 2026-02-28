// pkg/skills/cli/executor.go
package cli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// CLIExecutor executes shell commands securely
// This is NOT a connector - it's a command execution engine
type CLIExecutor struct {
	config *ExecutorConfig
	ctx    context.Context
	cancel context.CancelFunc
}

// ExecutorConfig holds CLI executor configuration
type ExecutorConfig struct {
	// Security settings
	AllowedShells    []string `yaml:"allowed_shells"`     // Default: ["bash", "sh"]
	MaxOutputSize    int      `yaml:"max_output_size"`    // Bytes, default: 1MB
	MaxExecutionTime int      `yaml:"max_execution_time"` // Seconds, default: 60
	WorkingDir       string   `yaml:"working_dir"`        // Default: /tmp

	// Sandboxing (future)
	UseSandbox  bool   `yaml:"use_sandbox"`
	SandboxType string `yaml:"sandbox_type"` // docker, firejail, etc.
}

// ExecutionResult holds command execution result
type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// NewCLIExecutor creates a CLI executor
func NewCLIExecutor(ctx context.Context, config *ExecutorConfig) *CLIExecutor {
	if config == nil {
		config = &ExecutorConfig{
			AllowedShells:    []string{"bash", "sh"},
			MaxOutputSize:    1024 * 1024, // 1MB
			MaxExecutionTime: 60,          // 60 seconds
			WorkingDir:       "/tmp",
		}
	}

	execCtx, cancel := context.WithCancel(ctx)

	return &CLIExecutor{
		config: config,
		ctx:    execCtx,
		cancel: cancel,
	}
}

// Execute executes a shell command
func (e *CLIExecutor) Execute(ctx context.Context, command string, timeoutSec int) (*ExecutionResult, error) {
	// Apply timeout
	if timeoutSec == 0 {
		timeoutSec = e.config.MaxExecutionTime
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Pre-execution validation
	if err := e.validateCommand(command); err != nil {
		logger.Log.Error("CLI command validation failed",
			"command", command,
			"error", err)
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Log execution
	logger.Log.Info("executing CLI command",
		"command", command,
		"timeout_sec", timeoutSec)

	// Determine shell
	shell := e.config.AllowedShells[0] // Use first allowed shell

	// Create command
	cmd := exec.CommandContext(execCtx, shell, "-c", command) //nolint:gosec
	cmd.Dir = e.config.WorkingDir

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute
	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Execution error (not exit code)
			logger.Log.Error("CLI command execution error",
				"command", command,
				"error", err)
			return nil, fmt.Errorf("execution error: %w", err)
		}
	}

	// Check output size
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	if len(stdoutStr) > e.config.MaxOutputSize {
		stdoutStr = stdoutStr[:e.config.MaxOutputSize] + "\n[OUTPUT TRUNCATED]"
	}
	if len(stderrStr) > e.config.MaxOutputSize {
		stderrStr = stderrStr[:e.config.MaxOutputSize] + "\n[OUTPUT TRUNCATED]"
	}

	// Log result
	if exitCode == 0 {
		logger.Log.Info("CLI command completed successfully",
			"command", command,
			"duration_ms", duration.Milliseconds(),
			"stdout_size", len(stdoutStr),
			"stderr_size", len(stderrStr))
	} else {
		logger.Log.Warn("CLI command failed",
			"command", command,
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

// Shutdown stops the executor
func (e *CLIExecutor) Shutdown() error {
	e.cancel()
	return nil
}
