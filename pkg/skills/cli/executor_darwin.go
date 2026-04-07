//go:build darwin

package cli

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// prepareCmd wraps the command with sandbox-exec on Darwin.
//
// sandbox-exec is technically deprecated by Apple but remains functional on
// all current macOS versions. It provides kernel-enforced filesystem and
// network restrictions via a TinyScheme policy profile.
//
// Instead of running: mkdir -p /path
// We run: sandbox-exec -p '(profile...)' mkdir -p /path
//
// In native mode the command is returned unwrapped.
func prepareCmd(ctx context.Context, binary string, args []string, security config.SecurityConfig) (*exec.Cmd, error) {
	if security.ExecutionMode == "native" {
		return exec.CommandContext(ctx, binary, args...), nil //nolint:gosec
	}

	profile := buildSandboxProfile(security)

	// sandbox-exec argv: sandbox-exec -p <profile> <binary> [args...]
	sandboxArgs := append([]string{"-p", profile, binary}, args...)
	cmd := exec.CommandContext(ctx, "sandbox-exec", sandboxArgs...) //nolint:gosec

	logger.Log.Debug("sandbox-exec profile built",
		"binary", binary,
		"allow_network", security.AllowNetwork,
		"workdir", security.WorkingDir)

	return cmd, nil
}

// runIsolated on Darwin just runs the command — sandbox-exec already
// provides the isolation layer via prepareCmd.
func runIsolated(_ context.Context, cmd *exec.Cmd, _ config.SecurityConfig) error {
	return cmd.Run()
}

// buildSandboxProfile generates a TinyScheme sandbox policy.
//
// Policy design:
//   - Default deny everything
//   - Allow reads from system paths (binaries, libs, locale data)
//   - Allow full read/write only within WorkingDir
//   - Allow or deny network based on AllowNetwork
//   - Allow process operations needed for normal execution
func buildSandboxProfile(security config.SecurityConfig) string {
	var b strings.Builder

	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")

	// Process operations — always allowed
	b.WriteString("(allow process*)\n")
	b.WriteString("(allow signal)\n")

	// System reads — needed for any binary to execute
	systemReadPaths := []string{
		"/usr/lib",
		"/usr/local/lib",
		"/System/Library",
		"/private/var/db/timezone",
		"/usr/share",
		"/dev/null",
		"/dev/urandom",
	}
	for _, p := range systemReadPaths {
		fmt.Fprintf(&b, "(allow file-read* (subpath %q))\n", p)
	}

	if security.AllowSystemBins {
		binPaths := []string{"/usr/bin", "/usr/local/bin", "/bin", "/usr/sbin", "/sbin"}
		for _, p := range binPaths {
			fmt.Fprintf(&b, "(allow file-read* (subpath %q))\n", p)
		}
	}

	// Sandbox root — full read/write within workdir only
	fmt.Fprintf(&b, "(allow file-read* file-write* (subpath %q))\n", security.WorkingDir)

	// Network
	if security.AllowNetwork {
		b.WriteString("(allow network*)\n")
	} else {
		b.WriteString("(deny network*)\n")
	}

	return b.String()
}
