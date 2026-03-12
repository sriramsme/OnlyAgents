package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/charmbracelet/huh"
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
	skillCmd.AddCommand(skillEditCmd)
	skillCmd.AddCommand(skillToolsCmd)

	skillToolsCmd.Flags().BoolP("commands", "c", false, "Show command templates")
	skillToolsCmd.Flags().String("access", "", "Filter by access level (read, write, admin)")
	skillToolsCmd.Flags().BoolP("verbose", "v", false, "Show parameters, timeout, and full descriptions")
	skillListCmd.Flags().String("access", "", "Filter by access level (read, write, admin)")
	skillListCmd.Flags().Bool("enabled", false, "Show only enabled skills")
	skillViewCmd.Flags().Bool("raw", false, "Dump raw file content")
	skillViewCmd.Flags().String("field", "", "Print a specific frontmatter field")
}

// ── list ──────────────────────────────────────────────────────────────────────

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

		accessFilter, err := cmd.Flags().GetString("access")
		if err != nil {
			return err
		}
		enabledOnly, err := cmd.Flags().GetBool("enabled")
		if err != nil {
			return err
		}

		if len(skills) == 0 {
			fmt.Println(cmdutil.StyleDim.Render("No skills found."))
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, cmdutil.StyleHeader.Render("NAME\tTYPE\tACCESS\tSTATUS\tDESCRIPTION"))
		fmt.Fprintln(w, "────\t────\t──────\t──────\t───────────")

		for _, s := range skills {
			if enabledOnly && !s.Enabled {
				continue
			}
			if accessFilter != "" && s.AccessLevel != accessFilter {
				continue
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				s.Name,
				s.Type,
				accessLabel(s.AccessLevel),
				cmdutil.EnabledLabel(s.Enabled),
				cmdutil.Truncate(s.Description, 50),
			)
		}
		return w.Flush()
	},
}

// ── enable / disable ──────────────────────────────────────────────────────────

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

// ── view ──────────────────────────────────────────────────────────────────────

var skillViewCmd = &cobra.Command{
	Use:   "view <name>",
	Short: "View a skill",
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
		s, err := cmdutil.FindSkill(skills, args[0])
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
		return cmdutil.ViewResource(cmdutil.SkillConfigPath(paths.Skills, args[0]), s, field, raw)
	},
}

// ── edit ──────────────────────────────────────────────────────────────────────

var skillEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit a skill's metadata interactively",
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
		s, err := cmdutil.FindSkill(skills, args[0])
		if err != nil {
			return err
		}

		enabled := s.Enabled
		accessLevel := s.AccessLevel
		if accessLevel == "" {
			accessLevel = "read"
		}

		accessOpts := []huh.Option[string]{
			huh.NewOption("read  — retrieves or lists data only", "read"),
			huh.NewOption("write — creates or updates data", "write"),
			huh.NewOption("admin — destructive or irreversible operations", "admin"),
		}

		if err := cmdutil.RunForm(
			huh.NewGroup(
				cmdutil.ConfirmField("Enabled", &enabled),
				cmdutil.SelectField("Access level", accessOpts, &accessLevel),
			),
		); err != nil {
			return err
		}

		if err := cmdutil.SkillSetEnabled(paths.Skills, args[0], enabled); err != nil {
			return err
		}
		if err := cmdutil.SkillSetAccessLevel(paths.Skills, args[0], accessLevel); err != nil {
			return err
		}

		cmdutil.Success("skill %s updated", args[0])
		return nil
	},
}

var skillToolsCmd = &cobra.Command{
	Use:   "tools <name>",
	Short: "List tools provided by a skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := resolvePaths()
		if err != nil {
			return err
		}
		accessFilter, err := cmd.Flags().GetString("access")
		if err != nil {
			return err
		}
		verbose, err := cmd.Flags().GetBool("verbose")
		if err != nil {
			return err
		}
		commands, err := cmd.Flags().GetBool("commands")
		if err != nil {
			return err
		}
		skill, err := cmdutil.LoadSkillWithTools(paths.Skills, args[0])
		if err != nil {
			return err
		}

		if len(skill.Commands) == 0 {
			fmt.Println(cmdutil.StyleDim.Render("No tools found in this skill."))
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

		if verbose {
			fmt.Fprintln(w, cmdutil.StyleHeader.Render("TOOL\tACCESS\tTIMEOUT\tPARAMS\tDESCRIPTION"))
			fmt.Fprintln(w, "────\t──────\t───────\t──────\t───────────")
		} else {
			fmt.Fprintln(w, cmdutil.StyleHeader.Render("TOOL\tACCESS\tDESCRIPTION"))
			fmt.Fprintln(w, "────\t──────\t───────────")
		}

		for _, c := range skill.Commands {
			if accessFilter != "" && c.Access != accessFilter {
				continue
			}
			if verbose {
				params := fmt.Sprintf("%d", len(c.ParamDefs))
				timeout := fmt.Sprintf("%ds", c.Timeout)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					cmdutil.StyleBold.Render(c.Name),
					accessLabel(c.Access),
					timeout,
					params,
					cmdutil.Truncate(c.Description, 50),
				)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%s\n",
					cmdutil.StyleBold.Render(c.Name),
					accessLabel(c.Access),
					cmdutil.Truncate(c.Description, 60),
				)
			}

			// inside the loop, after printing the tool row:
			if commands {
				fmt.Fprintf(w, "  %s\n", cmdutil.StyleDim.Render("$ "+c.Template))
			}

			// Under verbose, also print each param on its own line
			if verbose {
				for _, p := range c.ParamDefs {
					fmt.Fprintf(w, "  ↳ %s\t%s\t\t\t(%s)\n",
						p.Name, p.Type, cmdutil.Truncate(p.Description, 40))
				}
			}
		}

		if err := w.Flush(); err != nil {
			return err
		}

		fmt.Printf("\n%s\n", cmdutil.StyleDim.Render(
			fmt.Sprintf("%d tool(s) in %s skill", len(skill.Commands), args[0]),
		))
		return nil
	},
}

func init() {
	// add inside existing init()
}

// ── helpers ───────────────────────────────────────────────────────────────────

func accessLabel(level string) string {
	switch level {
	case "write":
		return cmdutil.StyleYellow.Render("write")
	case "admin":
		return cmdutil.StyleRed.Render("admin")
	default:
		return cmdutil.StyleDim.Render("read")
	}
}
