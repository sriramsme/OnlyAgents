package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/skills/reminders"
	"github.com/sriramsme/OnlyAgents/pkg/skills/runner"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func main() {
	runner.Run(
		"reminders",
		"Create, read, update, and manage reminders",
		tools.GetRemindersEntries(),
		func(root *cobra.Command) {
			root.PersistentFlags().String("connector", "local", "connector: local, google-calendar, ...")
			root.PersistentFlags().String("db", "main.db", "SQLite path (local connector)")
		},
		func(root *cobra.Command) (skills.Skill, error) {
			connector, err := root.PersistentFlags().GetString("connector")
			if err != nil {
				return nil, err
			}

			dbPath, err := root.PersistentFlags().GetString("db")
			if err != nil {
				return nil, err
			}

			conn, err := buildConnector(connector, dbPath)
			if err != nil {
				return nil, err
			}
			return reminders.New(context.Background(), conn)
		},
	)
}
