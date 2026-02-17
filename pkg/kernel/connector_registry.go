package kernel

import (
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
)

// ConnectorRegistry manages platform connectors
type ConnectorRegistry struct {
	connectors map[string]connectors.Connector
	mu         sync.RWMutex
}

func NewConnectorRegistry() *ConnectorRegistry {
	return &ConnectorRegistry{
		connectors: make(map[string]connectors.Connector),
	}
}

func (r *ConnectorRegistry) Register(connector connectors.Connector) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := connector.PlatformName()
	if _, exists := r.connectors[name]; exists {
		return fmt.Errorf("connector %s already registered", name)
	}

	r.connectors[name] = connector
	fmt.Printf("Registered connector: %s (v%s)\n", name, connector.Version())
	return nil
}

func (r *ConnectorRegistry) Get(name string) (connectors.Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	connector, exists := r.connectors[name]
	if !exists {
		return nil, fmt.Errorf("connector %s not found", name)
	}

	return connector, nil
}
