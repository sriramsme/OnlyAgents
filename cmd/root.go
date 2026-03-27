// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/paths"
)

var (
	cfgFile string
	verbose bool
	version = "dev"     // overridden by goreleaser
	commit  = "none"    // overridden by goreleaser
	date    = "unknown" // overridden by goreleaser
)

var rootCmd = &cobra.Command{
	Use:     "onlyagents",
	Short:   "OnlyAgents - only agents you need",
	Long:    `OnlyAgents is a flexible multi-agent framework. It is your personal assistant to everything in your life.`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Global flags available to all subcommands
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.onlyagents.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Optional: Hide completion command if you don't need it
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

func resolvePaths() (*paths.Paths, error) {
	p, err := paths.Init()
	if err != nil {
		return nil, fmt.Errorf("resolve paths: %w", err)
	}
	return p, nil
}

// # Run server
// onlyagents server start
// onlyagents server start --port 9000 --log-level info
//
// # Run agent kernel only
// onlyagents agent run
// onlyagents agent run --agents-dir ./my-agents --log-format text
//
// # Models commands
// onlyagents models list
// onlyagents models info gpt-5-nano
