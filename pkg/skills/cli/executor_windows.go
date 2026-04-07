//go:build windows

package cli

import (
	"context"
	"os/exec"

	"github.com/sriramsme/OnlyAgents/internal/config"
)

func prepareCmd(ctx context.Context, binary string, args []string, _ config.SecurityConfig) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, binary, args...), nil
}

func runIsolated(_ context.Context, cmd *exec.Cmd, _ config.SecurityConfig) error {
	return cmd.Run()
}
