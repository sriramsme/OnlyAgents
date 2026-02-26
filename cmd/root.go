// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
	version = "0.1.0" // Could be set via ldflags during build
)

var rootCmd = &cobra.Command{
	Use:     "onlyagents",
	Short:   "OnlyAgents - only agents you need",
	Long:    `OnlyAgents is a flexible multi-agent framework. It is your personal assistant to everything in your life.`,
	Version: version, // Adds automatic --version flag
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
