package main

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/connectors/gmail"
)

func init() {
	registry.Register("gmail", gmail.Config{}, func(cfg any) (connectors.EmailConnector, error) {
		return gmail.New(context.Background(), cfg.(gmail.Config))
	})
}
