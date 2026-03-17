// go build -tags connector_perplexity
package main

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/connectors/perplexity"
)

func init() {
	// override or extend buildConnector via a registry
	registerConnector("perplexity", func(cfg any) (connectors.WebSearchConnector, error) {
		var ddgCfg perplexity.Config
		if cfg != nil {
			ddgCfg = cfg.(perplexity.Config)
		}
		return perplexity.New(context.Background(), ddgCfg)
	})
}
