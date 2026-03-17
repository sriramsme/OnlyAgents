package main

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/skills/runner"
	"github.com/sriramsme/OnlyAgents/pkg/skills/websearch"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

var registry = runner.NewConnectorRegistry[connectors.WebSearchConnector]("duckduckgo")

func main() {
	runner.Run(
		"websearch",
		"Search and fetch the web",
		tools.GetWebSearchEntries(),
		registry.RegisterFlags,
		func(root *cobra.Command) (skills.Skill, error) {
			connector, err := root.PersistentFlags().GetString("connector")
			if err != nil {
				return nil, err
			}

			conn, err := registry.Build(connector, root)
			if err != nil {
				return nil, err
			}
			return websearch.New(context.Background(), conn)
		},
	)
}
