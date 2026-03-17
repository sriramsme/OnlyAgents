package main

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/connectors/duckduckgo"
)

func init() {
	// override or extend buildConnector via a registry
	registerConnector("duckduckgo", func(cfg any) (connectors.WebSearchConnector, error) {
		var ddgCfg duckduckgo.Config
		if cfg != nil {
			ddgCfg = cfg.(duckduckgo.Config)
		}
		return duckduckgo.New(context.Background(), ddgCfg)
	})
}
