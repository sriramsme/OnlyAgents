// go build -tags connector_perplexity
package main

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/connectors/perplexity"
)

func init() {
	registry.Register("perplexity", perplexity.Config{}, func(cfg any) (connectors.WebSearchConnector, error) {
		return perplexity.New(context.Background(), cfg.(perplexity.Config))
	})
}
