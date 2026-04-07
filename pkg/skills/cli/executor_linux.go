//go:build linux

package cli

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/landlock-lsm/go-landlock/landlock"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// prepareCmd returns a plain exec.Cmd on Linux.
// Isolation is applied in runIsolated via Landlock on a locked OS thread.
func prepareCmd(ctx context.Context, binary string, args []string, _ config.SecurityConfig) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, binary, args...), nil //nolint:gosec
}

// runIsolated runs cmd under Landlock filesystem restrictions on Linux.
//
// Landlock restricts the calling thread. To avoid restricting the main
// OnlyAgents process, we lock a dedicated OS thread, apply Landlock to it,
// then exec the child from that thread. When the goroutine exits the locked
// OS thread is terminated by the Go runtime — the restriction never leaks
// back to the parent.
//
// In native mode or if the kernel predates Landlock (< 5.13), execution
// falls back to an unrestricted run with a warning.
func runIsolated(ctx context.Context, cmd *exec.Cmd, security config.SecurityConfig) error {
	if security.ExecutionMode == "native" {
		return cmd.Run()
	}

	type result struct{ err error }
	ch := make(chan result, 1)

	go func() {
		// Lock this goroutine to a single OS thread. Landlock applies to the
		// thread, not the goroutine. Without LockOSThread the Go scheduler
		// could migrate us to another thread mid-call.
		// Intentionally no UnlockOSThread — the runtime terminates a locked
		// thread when its goroutine exits, preventing restriction leakage.
		runtime.LockOSThread()

		rules := buildLandlockRules(security)
		if err := landlock.V3.BestEffort().RestrictPaths(rules...); err != nil {
			// BestEffort means older kernels degrade gracefully instead of
			// failing hard. An error here is unexpected but non-fatal.
			logger.Log.Warn("landlock restriction failed, running unrestricted",
				"err", err)
		}

		// Requires kernel support; silently ignored if unavailable.
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Pdeathsig: syscall.SIGKILL, // child dies if parent dies
		}

		ch <- result{cmd.Run()}
	}()

	select {
	case r := <-ch:
		return r.err
	case <-ctx.Done():
		if cmd.Process != nil {
			err := cmd.Process.Kill()
			if err != nil {
				logger.Log.Warn("failed to kill child process", "err", err)
			}
		}
		<-ch // drain so the goroutine can exit
		return ctx.Err()
	}
}

func buildLandlockRules(security config.SecurityConfig) []landlock.Rule {
	rules := []landlock.Rule{
		// Workspace access
		landlock.RWDirs(security.WorkingDir),

		// REQUIRED: allow stdio
		landlock.RWFiles("/dev/null"),
	}

	if security.AllowSystemBins {
		rules = append(rules,
			landlock.RODirs(
				"/usr/bin",
				"/usr/local/bin",
				"/bin",
				"/lib",
				"/lib64",
				"/usr/lib",
				"/usr/local/lib",
			),

			// Strongly recommended for curl / TLS / DNS
			landlock.ROFiles(
				"/etc/hosts",
				"/etc/resolv.conf",
			),
			landlock.RODirs(
				"/etc/ssl",
				"/etc/ssl/certs",
			),

			// Crypto randomness (curl will need this)
			landlock.ROFiles("/dev/urandom"),
		)
	}

	return rules
}

// buildLandlockRules constructs path rules from SecurityConfig.
// Note: network namespace isolation (AllowNetwork: false) requires
// CLONE_NEWNET which needs CAP_SYS_ADMIN or a user namespace wrapper.
// That complexity is deferred — workdir + Landlock covers the primary
// threat model for a personal self-hosted tool.
var _ = fmt.Sprintf // suppress unused import if landlock import shifts
