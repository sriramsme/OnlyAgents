package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/cmdutil"
)

var connectorCmd = &cobra.Command{
	Use:   "connector",
	Short: "Manage connectors",
}

func init() {
	rootCmd.AddCommand(connectorCmd)
	connectorCmd.AddCommand(connectorListCmd)
	connectorCmd.AddCommand(connectorSetupCmd)
	connectorCmd.AddCommand(connectorEnableCmd)
	connectorCmd.AddCommand(connectorDisableCmd)
	connectorCmd.AddCommand(connectorViewCmd)
	connectorCmd.AddCommand(connectorEditCmd)

	connectorViewCmd.Flags().String("field", "", "Print a specific field value")
	connectorViewCmd.Flags().Bool("raw", false, "Dump raw YAML")
}

var connectorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured connectors",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		connectors, err := cmdutil.ConnectorRegistry(paths.Connectors)
		if err != nil {
			return err
		}
		if len(connectors) == 0 {
			fmt.Println(cmdutil.StyleDim.Render("No connectors configured."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, cmdutil.StyleHeader.Render("NAME\tID\tSTATUS\tDESCRIPTION"))
		fmt.Fprintln(w, "────\t────\t──────\t───────────")
		for _, c := range connectors {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				c.Name,
				c.ID,
				cmdutil.EnabledLabel(c.Enabled),
				cmdutil.Truncate(c.Description, 50),
			)
		}
		return w.Flush()
	},
}

var connectorSetupCmd = &cobra.Command{
	Use:   "setup <name>",
	Short: "Run interactive setup for a connector",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		connectors, err := cmdutil.ConnectorRegistry(paths.Connectors)
		if err != nil {
			return err
		}
		cfg, err := cmdutil.FindConnector(connectors, args[0])
		if err != nil {
			return err
		}
		return cmdutil.SetupConnector(cfg, paths.EnvPath, paths.Connectors)
	},
}

var connectorEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a connector",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		if err := cmdutil.ConnectorSetEnabled(paths.Connectors, args[0], true); err != nil {
			return err
		}
		cmdutil.Success("%s enabled", args[0])
		return nil
	},
}

var connectorDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a connector",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		if err := cmdutil.ConnectorSetEnabled(paths.Connectors, args[0], false); err != nil {
			return err
		}
		cmdutil.Warn("%s disabled", args[0])
		return nil
	},
}

var connectorViewCmd = &cobra.Command{
	Use:   "view <name>",
	Short: "View a connector config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		connectors, err := cmdutil.ConnectorRegistry(paths.Connectors)
		if err != nil {
			return err
		}
		cfg, err := cmdutil.FindConnector(connectors, args[0])
		if err != nil {
			return err
		}
		raw, err := cmd.Flags().GetBool("raw")
		if err != nil {
			return err
		}
		field, err := cmd.Flags().GetString("field")
		if err != nil {
			return err
		}
		return cmdutil.ViewResource(cmdutil.ConnectorConfigPath(paths.Connectors, args[0]), cfg, field, raw)
	},
}

var connectorEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit a connector config interactively",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		connectors, err := cmdutil.ConnectorRegistry(paths.Connectors)
		if err != nil {
			return err
		}
		cfg, err := cmdutil.FindConnector(connectors, args[0])
		if err != nil {
			return err
		}

		name := cfg.Name
		enabled := cfg.Enabled

		if err := cmdutil.RunForm(
			huh.NewGroup(
				cmdutil.InputField("Name", cfg.Name, &name),
				cmdutil.ConfirmField("Enabled", &enabled),
			),
		); err != nil {
			return err
		}

		path := cmdutil.ConnectorConfigPath(paths.Connectors, args[0])
		var raw map[string]any
		if err := cmdutil.ReadYAML(path, &raw); err != nil {
			return err
		}
		raw["name"] = name
		raw["enabled"] = enabled

		if err := cmdutil.WriteYAML(path, raw); err != nil {
			return err
		}
		cmdutil.Success("connector %s updated", args[0])
		return nil
	},
}
