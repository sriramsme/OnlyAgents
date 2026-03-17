// go build -tags connector_brave
package main

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/connectors/brave"
)

func init() {
	registry.Register("brave", brave.Config{}, func(cfg any) (connectors.WebSearchConnector, error) {
		return brave.New(context.Background(), cfg.(brave.Config))
	})
}
