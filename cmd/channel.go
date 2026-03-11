package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/cmdutil"
)

var channelCmd = &cobra.Command{
	Use:   "channel",
	Short: "Manage channels",
}

func init() {
	rootCmd.AddCommand(channelCmd)
	channelCmd.AddCommand(channelListCmd)
	channelCmd.AddCommand(channelSetupCmd)
	channelCmd.AddCommand(channelEnableCmd)
	channelCmd.AddCommand(channelDisableCmd)
	channelCmd.AddCommand(channelViewCmd)
	channelCmd.AddCommand(channelEditCmd)

	channelViewCmd.Flags().String("field", "", "Print a specific field value")
	channelViewCmd.Flags().Bool("raw", false, "Dump raw YAML")
}

// ── list ──────────────────────────────────────────────────────────────────────

var channelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured channels",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		channels, err := cmdutil.ChannelRegistry(paths.Channels)
		if err != nil {
			return err
		}
		if len(channels) == 0 {
			fmt.Println(cmdutil.StyleDim.Render("No channels configured."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, cmdutil.StyleHeader.Render("NAME\tPLATFORM\tSTATUS\tDESCRIPTION"))
		fmt.Fprintln(w, "────\t────────\t──────\t───────────")
		for _, c := range channels {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				c.Name,
				c.Platform,
				cmdutil.EnabledLabel(c.Enabled),
				cmdutil.Truncate(c.Description, 50),
			)
		}
		return w.Flush()
	},
}

// ── setup ─────────────────────────────────────────────────────────────────────

var channelSetupCmd = &cobra.Command{
	Use:   "setup <platform>",
	Short: "Run interactive setup for a channel",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		channels, err := cmdutil.ChannelRegistry(paths.Channels)
		if err != nil {
			return err
		}
		cfg, err := cmdutil.FindChannelByPlatform(channels, args[0])
		if err != nil {
			return err
		}
		return cmdutil.SetupChannel(cfg, paths.EnvPath, paths.Channels)
	},
}

// ── enable ────────────────────────────────────────────────────────────────────

var channelEnableCmd = &cobra.Command{
	Use:   "enable <platform>",
	Short: "Enable a channel",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		if err := cmdutil.ChannelSetEnabled(paths.Channels, args[0], true); err != nil {
			return err
		}
		cmdutil.Success("%s enabled", args[0])
		return nil
	},
}

// ── disable ───────────────────────────────────────────────────────────────────

var channelDisableCmd = &cobra.Command{
	Use:   "disable <platform>",
	Short: "Disable a channel",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		if err := cmdutil.ChannelSetEnabled(paths.Channels, args[0], false); err != nil {
			return err
		}
		cmdutil.Warn("%s disabled", args[0])
		return nil
	},
}

// ── view ──────────────────────────────────────────────────────────────────────

var channelViewCmd = &cobra.Command{
	Use:   "view <platform>",
	Short: "View a channel config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		channels, err := cmdutil.ChannelRegistry(paths.Channels)
		if err != nil {
			return err
		}
		cfg, err := cmdutil.FindChannelByPlatform(channels, args[0])
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

		return cmdutil.ViewResource(cmdutil.ChannelConfigPath(paths.Channels, args[0]), cfg, field, raw)
	},
}

// ── edit ──────────────────────────────────────────────────────────────────────

var channelEditCmd = &cobra.Command{
	Use:   "edit <platform>",
	Short: "Edit a channel config interactively",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		channels, err := cmdutil.ChannelRegistry(paths.Channels)
		if err != nil {
			return err
		}
		cfg, err := cmdutil.FindChannelByPlatform(channels, args[0])
		if err != nil {
			return err
		}

		name := cfg.Name
		desc := cfg.Description
		enabled := cfg.Enabled
		priority := fmt.Sprintf("%d", cfg.Priority)

		if err := cmdutil.RunForm(
			huh.NewGroup(
				cmdutil.InputField("Name", cfg.Name, &name),
				cmdutil.InputField("Description", "", &desc),
				cmdutil.InputField("Priority", "0", &priority),
				cmdutil.ConfirmField("Enabled", &enabled),
			),
		); err != nil {
			return err
		}

		path := cmdutil.ChannelConfigPath(paths.Channels, args[0])
		var raw map[string]any
		if err := cmdutil.ReadYAML(path, &raw); err != nil {
			return err
		}
		raw["name"] = name
		raw["description"] = desc
		raw["enabled"] = enabled
		raw["priority"] = priority

		if err := cmdutil.WriteYAML(path, raw); err != nil {
			return err
		}
		cmdutil.Success("channel %s updated", args[0])
		return nil
	},
}
