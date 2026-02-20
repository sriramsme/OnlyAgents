package kernel

import (
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
)

// Factory creates a connector from raw config
type ConnectorFactory func(
	rawConfig map[string]interface{},
	vault vault.Vault,
	agentRegistry *AgentRegistry,
) (connectors.Connector, error)

var (
	connectorFactoryMu sync.RWMutex
	connectorFactories = make(map[string]ConnectorFactory)
)

// Register registers a connector factory for a platform
func RegisterConnector(platform string, factory ConnectorFactory) {
	connectorFactoryMu.Lock()
	defer connectorFactoryMu.Unlock()

	if factory == nil {
		panic("connectors: Register factory is nil for platform " + platform)
	}
	if _, exists := connectorFactories[platform]; exists {
		panic("connectors: Register called twice for platform " + platform)
	}

	connectorFactories[platform] = factory
}

// GetFactory returns the factory for a platform
func GetConnectorFactory(platform string) (ConnectorFactory, error) {
	connectorFactoryMu.RLock()
	defer connectorFactoryMu.RUnlock()

	factory, ok := connectorFactories[platform]
	if !ok {
		return nil, fmt.Errorf("no factory registered for platform: %s", platform)
	}

	return factory, nil
}

// ListRegistered returns all registered platform names
func ListRegisteredConnectors() []string {
	connectorFactoryMu.RLock()
	defer connectorFactoryMu.RUnlock()

	platforms := make([]string, 0, len(connectorFactories))
	for platform := range connectorFactories {
		platforms = append(platforms, platform)
	}
	return platforms
}
