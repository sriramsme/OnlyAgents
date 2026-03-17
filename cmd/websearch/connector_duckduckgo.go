package main

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/connectors/duckduckgo"
)

func init() {
	registry.Register("duckduckgo", duckduckgo.Config{}, func(cfg any) (connectors.WebSearchConnector, error) {
		return duckduckgo.New(context.Background(), cfg.(duckduckgo.Config))
	})
}
