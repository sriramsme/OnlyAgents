package kernel

import (
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
)

// Factory creates a connector from raw config
type ChannelFactory func(
	rawConfig map[string]interface{},
	vault vault.Vault,
	agentRegistry *AgentRegistry,
) (channels.Channel, error)

var (
	channelFactoryMu sync.RWMutex
	channelFactories = make(map[string]ChannelFactory)
)

// Register registers a connector factory for a platform
func RegisterChannel(platform string, factory ChannelFactory) {
	channelFactoryMu.Lock()
	defer channelFactoryMu.Unlock()

	if factory == nil {
		panic("connectors: Register factory is nil for platform " + platform)
	}
	if _, exists := channelFactories[platform]; exists {
		panic("connectors: Register called twice for platform " + platform)
	}

	channelFactories[platform] = factory
}

// GetFactory returns the factory for a platform
func GetChannelFactory(platform string) (ChannelFactory, error) {
	channelFactoryMu.RLock()
	defer channelFactoryMu.RUnlock()

	factory, ok := channelFactories[platform]
	if !ok {
		return nil, fmt.Errorf("no factory registered for platform: %s", platform)
	}

	return factory, nil
}

// ListRegistered returns all registered platform names
func ListRegisteredChannels() []string {
	channelFactoryMu.RLock()
	defer channelFactoryMu.RUnlock()

	platforms := make([]string, 0, len(channelFactories))
	for platform := range channelFactories {
		platforms = append(platforms, platform)
	}
	return platforms
}
