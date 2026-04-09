package main

import (
	"fmt"

	"github.com/sriramsme/OnlyAgents/internal/storage/sqlite"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/connectors/local"
)

var connectorRegistry = map[string]func() (connectors.RemindersConnector, error){}

// func registerConnector(name string, fn func() (connectors.RemindersConnector, error)) {
// 	connectorRegistry[name] = fn
// }

func buildConnector(name string, dbPath string) (connectors.RemindersConnector, error) {
	// local is always available
	if name == "local" || name == "" {
		store, err := sqlite.NewRemindersStore(dbPath)
		if err != nil {
			return nil, err
		}
		return local.NewRemindersConnector(store), nil
	}
	fn, ok := connectorRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown connector %q — rebuild with appropriate build tag", name)
	}
	return fn()
}
