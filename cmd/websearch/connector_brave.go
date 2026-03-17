// go build -tags connector_brave
package main

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/connectors/brave"
)

func init() {
	// override or extend buildConnector via a registry
	registerConnector("brave", func(cfg any) (connectors.WebSearchConnector, error) {
		var ddgCfg brave.Config
		if cfg != nil {
			ddgCfg = cfg.(brave.Config)
		}
		return brave.New(context.Background(), ddgCfg)
	})
}
