package main

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/skills/notes"
	"github.com/sriramsme/OnlyAgents/pkg/skills/runner"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func main() {
	runner.Run(
		"notes",
		"Create, read, update, and manage notes",
		tools.GetNotesEntries(),
		func(root *cobra.Command) {
			root.PersistentFlags().String("connector", "local", "connector: local, notion, ...")
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
			return notes.New(context.Background(), conn)
		},
	)
}
