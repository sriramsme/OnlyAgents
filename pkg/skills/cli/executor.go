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
	"github.com/sriramsme/OnlyAgents/pkg/skills"
)

// CLIExecutor executes structured commands — no shell, no template rendering.
// Binary and args are constructed by CLISkill before reaching here.
// OS-level isolation is applied per platform via prepareCmd / runIsolated.
type CLIExecutor struct {
	execCfg     *skills.ExecutorConfig
	security    config.SecurityConfig
	requiredEnv []string
}

type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

func NewCLIExecutor(
	execCfg *skills.ExecutorConfig,
	security config.SecurityConfig,
	requiredEnv []string,
) *CLIExecutor {
	return &CLIExecutor{
		execCfg:     execCfg,
		security:    security,
		requiredEnv: requiredEnv,
	}
}

// Execute runs binary with args. stdin is optional — set it when a tool needs
// to pipe content (e.g. tee for fs_write). outputDir is scanned for produced
// files after execution.
func (e *CLIExecutor) Execute(
	ctx context.Context,
	binary string,
	args []string,
	stdin string,
	timeoutSec int,
	outputDir string,
) (*ExecutionResult, error) {
	if err := e.validateWorkingDir(); err != nil {
		return nil, err
	}

	timeoutSec = e.resolveTimeout(timeoutSec)
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	if err := os.MkdirAll(filepath.Join(e.security.WorkingDir, "tmp"), 0o700); err != nil {
		return nil, fmt.Errorf("create tmp dir: %w", err)
	}

	// prepareCmd is platform-specific: on Darwin it wraps with sandbox-exec,
	// on Linux it returns a plain exec.Cmd (isolation applied in runIsolated).
	cmd, err := prepareCmd(execCtx, binary, args, e.security)
	if err != nil {
		return nil, fmt.Errorf("prepare cmd: %w", err)
	}

	cmd.Dir = e.security.WorkingDir
	cmd.Env = e.buildEnv(outputDir)

	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	logger.Log.Info("executing CLI command",
		"binary", binary,
		"args", args,
		"timeout_sec", timeoutSec,
		"mode", e.security.ExecutionMode,
		"working_dir", e.security.WorkingDir)

	start := time.Now()
	// runIsolated is platform-specific: on Linux it applies Landlock before exec,
	// on Darwin sandbox-exec is already in the command, so it just calls cmd.Run.
	runErr := runIsolated(execCtx, cmd, e.security)
	duration := time.Since(start)

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if ok := isExitError(runErr, &exitErr); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("execution error: %w", runErr)
		}
	}

	maxOut := e.resolveMaxOutput()
	stdoutStr := sanitizeOutput(truncate(stdout.String(), maxOut))
	stderrStr := sanitizeOutput(truncate(stderr.String(), maxOut))

	if exitCode == 0 {
		logger.Log.Info("CLI command succeeded",
			"binary", binary,
			"duration_ms", duration.Milliseconds(),
			"stdout_bytes", len(stdoutStr))
	} else {
		logger.Log.Warn("CLI command failed",
			"binary", binary,
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

// ── Helpers ───────────────────────────────────────────────────────────────────

func (e *CLIExecutor) resolveTimeout(perCallSec int) int {
	if perCallSec > 0 {
		return perCallSec
	}
	if e.execCfg.MaxExecutionTime > 0 {
		return e.execCfg.MaxExecutionTime
	}
	return 60
}

func (e *CLIExecutor) resolveMaxOutput() int {
	if e.execCfg.MaxOutputSize > 0 {
		return e.execCfg.MaxOutputSize
	}
	return 1 << 20 // 1 MB default
}

func (e *CLIExecutor) validateWorkingDir() error {
	abs, err := filepath.Abs(e.security.WorkingDir)
	if err != nil {
		return fmt.Errorf("invalid working dir: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	allowed := filepath.Join(home, ".onlyagents")
	if !strings.HasPrefix(abs, allowed) {
		return fmt.Errorf("working dir %q is outside allowed root %q", abs, allowed)
	}
	return nil
}

func (e *CLIExecutor) buildEnv(outputDir string) []string {
	if e.security.ExecutionMode == "native" {
		env := os.Environ()
		return append(env, "OUTPUT_DIR="+outputDir)
	}

	// restricted mode — minimal env, no secret leakage
	base := []string{
		"HOME=" + e.security.WorkingDir,
		"TMPDIR=" + filepath.Join(e.security.WorkingDir, "tmp"),
		"OUTPUT_DIR=" + outputDir,
		"PATH=" + e.buildPath(),
		"TERM=dumb",
		"LANG=en_US.UTF-8",
	}

	// Only forward env vars the skill explicitly declared it needs
	for _, key := range e.requiredEnv {
		if val := os.Getenv(key); val != "" {
			base = append(base, key+"="+val)
		}
	}

	return base
}

func (e *CLIExecutor) buildPath() string {
	workBin := filepath.Join(e.security.WorkingDir, "bin")
	if !e.security.AllowSystemBins {
		return workBin
	}
	return workBin + ":/usr/local/bin:/usr/bin:/bin"
}

func isExitError(err error, out **exec.ExitError) bool {
	if err == nil {
		return false
	}
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		*out = exitErr
	}
	return ok
}

func truncate(s string, max int) string {
	if max > 0 && len(s) > max {
		return s[:max] + "\n[OUTPUT TRUNCATED]"
	}
	return s
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password)\s*[:=]\s*\S+`),
	regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`), // base64-encoded secrets
}

func sanitizeOutput(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}
