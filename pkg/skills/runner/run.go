// pkg/skills/runner/run.go
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// Run is the single entry point for all skill binaries.
// setup registers any global flags and returns the constructed skill.
func Run(
	name string,
	desc string,
	entries []tools.ToolEntry,
	registerGlobalFlags func(root *cobra.Command),
	setup func(root *cobra.Command) (skills.Skill, error),
) {
	var skill skills.Skill
	root := &cobra.Command{
		Use:   name,
		Short: desc,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			skill, err = setup(cmd.Root())
			return err
		},
	}
	if registerGlobalFlags != nil {
		registerGlobalFlags(root)
	}
	for _, entry := range entries {
		entry := entry
		sub := subcommandName(name, entry.Def.Name)
		cmd := &cobra.Command{
			Use:   sub,
			Short: entry.Def.Description,
			RunE: func(cmd *cobra.Command, args []string) error {
				payload, err := buildPayload(cmd, entry.Input, args)
				if err != nil {
					return err
				}
				exec := skill.Execute(context.Background(), entry.Def.Name, payload)
				if exec.Err != nil {
					return exec.Err
				}
				printResult(exec.Result)
				return nil
			},
		}
		addInputFlags(cmd, entry.Input)
		root.AddCommand(cmd)
	}
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// addInputFlags reflects over the input struct and adds cobra flags to a subcommand.
func addInputFlags(cmd *cobra.Command, proto any) {
	t := reflect.TypeOf(proto)
	var posField string

	for i := range t.NumField() {
		meta := tools.ParseFieldMeta(t.Field(i))
		if meta.JSONName == "" || meta.JSONName == "-" {
			continue
		}

		desc := meta.SchemaDesc
		if meta.CLIHelp != "" {
			desc += " (" + meta.CLIHelp + ")"
		}
		if meta.CLIPos != "" {
			posField = meta.JSONName
			desc += fmt.Sprintf(" (or positional arg #%s)", meta.CLIPos)
		}

		if meta.CLIShort != "" {
			cmd.Flags().StringP(meta.JSONName, meta.CLIShort, "", desc)
		} else {
			cmd.Flags().String(meta.JSONName, "", desc)
		}

		if meta.CLIRequired {
			err := cmd.MarkFlagRequired(meta.JSONName)
			if err != nil {
				panic(err)
			}
		}
	}

	if posField != "" {
		cmd.Annotations = map[string]string{"pos_field": posField}
	}
}

func printResult(result any) {
	switch v := result.(type) {
	case map[string]any:
		if summary, ok := v["summary"]; ok {
			fmt.Println(summary)
			return
		}
		for k, val := range v {
			fmt.Printf("%s: %v\n", k, val)
		}
	case string:
		fmt.Println(v)
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(string(b))
	}
}

func jsonFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		return strings.ToLower(f.Name)
	}
	return strings.Split(tag, ",")[0]
}

func subcommandName(skillName, toolName string) string {
	// try plural prefix: tasks_create → create
	if s := strings.TrimPrefix(toolName, skillName+"_"); s != toolName {
		return s
	}
	// try singular prefix: task_complete → complete
	if strings.HasSuffix(skillName, "s") {
		singular := strings.TrimSuffix(skillName, "s")
		if s := strings.TrimPrefix(toolName, singular+"_"); s != toolName {
			return s
		}
	}
	// no match — use as-is, replace underscores with dashes for readability
	return strings.ReplaceAll(toolName, "_", "-")
}
