package main

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/skills/email"
	"github.com/sriramsme/OnlyAgents/pkg/skills/runner"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

var registry = runner.NewConnectorRegistry[connectors.EmailConnector]("gmail")

func main() {
	runner.Run(
		"email",
		"Send, receive and do more with your emails",
		tools.GetEmailEntries(),
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
			return email.New(context.Background(), conn)
		},
	)
}
