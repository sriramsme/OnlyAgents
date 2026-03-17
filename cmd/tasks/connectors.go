package main

import (
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/connectors/local"
	"github.com/sriramsme/OnlyAgents/pkg/storage/sqlite"
)

var connectorRegistry = map[string]func() (connectors.TasksConnector, error){}

// func registerConnector(name string, fn func() (connectors.TasksConnector, error)) {
// 	connectorRegistry[name] = fn
// }

func buildConnector(name string, dbPath string) (connectors.TasksConnector, error) {
	// local is always available
	if name == "local" || name == "" {
		store, err := sqlite.NewTasksStore(dbPath)
		if err != nil {
			return nil, err
		}
		return local.NewTasksConnector(store), nil
	}
	fn, ok := connectorRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown connector %q — rebuild with appropriate build tag", name)
	}
	return fn()
}
