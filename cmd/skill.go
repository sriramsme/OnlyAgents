package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/cmdutil"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage skills",
}

func init() {
	rootCmd.AddCommand(skillCmd)
	skillCmd.AddCommand(skillListCmd)
	skillCmd.AddCommand(skillEnableCmd)
	skillCmd.AddCommand(skillDisableCmd)
	skillCmd.AddCommand(skillViewCmd)

	skillViewCmd.Flags().String("field", "", "Print a specific field value")
	skillViewCmd.Flags().Bool("raw", false, "Dump raw YAML")
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all skills",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		skills, err := cmdutil.SkillRegistry(paths.Skills)
		if err != nil {
			return err
		}
		if len(skills) == 0 {
			fmt.Println(cmdutil.StyleDim.Render("No skills configured."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, cmdutil.StyleHeader.Render("NAME\tCAPABILITIES\tENABLED"))
		fmt.Fprintln(w, "────\t────\t──────")
		for _, s := range skills {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				s.Name,
				s.Capabilities,
				cmdutil.EnabledLabel(s.Enabled),
			)
		}
		return w.Flush()
	},
}

var skillEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		if err := cmdutil.SkillSetEnabled(paths.Skills, args[0], true); err != nil {
			return err
		}
		cmdutil.Success("%s enabled", args[0])
		return nil
	},
}

var skillDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		if err := cmdutil.SkillSetEnabled(paths.Skills, args[0], false); err != nil {
			return err
		}
		cmdutil.Warn("%s disabled", args[0])
		return nil
	},
}

var skillViewCmd = &cobra.Command{
	Use:   "view <name>",
	Short: "View a skill config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		skills, err := cmdutil.SkillRegistry(paths.Skills)
		if err != nil {
			return err
		}
		cfg, err := cmdutil.FindSkill(skills, args[0])
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
		return cmdutil.ViewResource(cmdutil.SkillConfigPath(paths.Skills, args[0]), cfg, field, raw)
	},
}
