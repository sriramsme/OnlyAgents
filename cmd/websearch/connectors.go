package main

import (
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
)

var connectorRegistry = map[string]func(cfg any) (connectors.WebSearchConnector, error){}

func registerConnector(name string, fn func(cfg any) (connectors.WebSearchConnector, error)) {
	connectorRegistry[name] = fn
}

func buildConnector(name string, cfg any) (connectors.WebSearchConnector, error) {
	fn, ok := connectorRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown connector %q — rebuild with appropriate build tag", name)
	}
	return fn(cfg)
}
