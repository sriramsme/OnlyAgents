package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/cmdutil"
)

var councilCmd = &cobra.Command{
	Use:   "council",
	Short: "Manage agent councils (preconfigured teams)",
}

func init() {
	rootCmd.AddCommand(councilCmd)
	councilCmd.AddCommand(councilListCmd)
	councilCmd.AddCommand(councilInfoCmd)
	councilCmd.AddCommand(councilEnableCmd)
	councilCmd.AddCommand(councilDisableCmd)
}

// ── list ──────────────────────────────────────────────────────────────────────

var councilListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available councils",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		councils, err := cmdutil.CouncilRegistry(paths.Councils)
		if err != nil {
			return err
		}
		if len(councils) == 0 {
			fmt.Println(cmdutil.StyleDim.Render("No councils found."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, cmdutil.StyleHeader.Render("NAME\tSTATUS\tAGENTS\tSKILLS\tCONNECTORS\tDESCRIPTION"))
		fmt.Fprintln(w, "────\t──────\t──────\t──────\t──────────\t───────────")
		for _, c := range councils {
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%s\n",
				c.Name,
				cmdutil.EnabledLabel(c.Enabled),
				len(c.Agents),
				len(c.Skills),
				len(c.Connectors),
				cmdutil.Truncate(c.Description, 50),
			)
		}
		return w.Flush()
	},
}

// ── info ──────────────────────────────────────────────────────────────────────

var councilInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show council details and current component status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		councils, err := cmdutil.CouncilRegistry(paths.Councils)
		if err != nil {
			return err
		}
		cfg, err := cmdutil.FindCouncil(councils, args[0])
		if err != nil {
			return err
		}

		status := cmdutil.CouncilInfo(cfg, paths)

		fmt.Println(cmdutil.StyleBorder.Render(
			cmdutil.StyleHeader.Render(cfg.Name) + " — " + cfg.Description,
		))
		fmt.Printf("Status: %s\n\n", cmdutil.EnabledLabel(status.Active))

		printResourceTable := func(title string, items []cmdutil.ResourceStatus) {
			fmt.Println(cmdutil.StyleBold.Render(title))
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for _, r := range items {
				present := cmdutil.StyleGreen.Render("✓")
				if !r.Present {
					present = cmdutil.StyleRed.Render("✗ not installed")
				}
				fmt.Fprintf(w, "  %s\t%s\t%s\n",
					r.Name,
					present,
					cmdutil.EnabledLabel(r.Enabled),
				)
			}
			err := w.Flush()
			if err != nil {
				fmt.Println("error flushing table:", err)
			}
			fmt.Println()
		}

		printResourceTable("Agents", status.Agents)
		printResourceTable("Skills", status.Skills)
		printResourceTable("Connectors", status.Connectors)

		return nil
	},
}

// ── enable ────────────────────────────────────────────────────────────────────

var councilEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a council and all its components",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		councils, err := cmdutil.CouncilRegistry(paths.Councils)
		if err != nil {
			return err
		}
		cfg, err := cmdutil.FindCouncil(councils, args[0])
		if err != nil {
			return err
		}

		warnings := cmdutil.EnableCouncil(cfg, paths)
		for _, w := range warnings {
			cmdutil.Warn("%s", w)
		}

		cmdutil.Success("council %s enabled (%d agents, %d skills, %d connectors)",
			cfg.Name, len(cfg.Agents), len(cfg.Skills), len(cfg.Connectors))
		return nil
	},
}

// ── disable ───────────────────────────────────────────────────────────────────

var councilDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a council (shared resources are preserved)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		councils, err := cmdutil.CouncilRegistry(paths.Councils)
		if err != nil {
			return err
		}
		cfg, err := cmdutil.FindCouncil(councils, args[0])
		if err != nil {
			return err
		}

		warnings := cmdutil.DisableCouncil(cfg, paths)
		for _, w := range warnings {
			cmdutil.Warn("%s", w)
		}

		cmdutil.Warn("council %s disabled", cfg.Name)
		return nil
	},
}
