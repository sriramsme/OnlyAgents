package connectors

import "github.com/sriramsme/OnlyAgents/internal/config"

// BaseConnectorInfo holds identity info for a connector.
// Used by native connectors to self-describe without config dependency.
type BaseConnectorInfo struct {
	ID           string
	Name         string
	Description  string
	Instructions string
	Enabled      bool
	Type         ConnectorType
}

// BaseConnector provides common functionality for all connectors
type BaseConnector struct {
	id           string
	name         string
	description  string
	instructions string
	enabled      bool
	connType     ConnectorType
}

// NewBaseConnector creates a base connector from BaseConnectorInfo.
// Used directly by native connectors.
func NewBaseConnector(info BaseConnectorInfo) *BaseConnector {
	return &BaseConnector{
		id:           info.ID,
		name:         info.Name,
		description:  info.Description,
		instructions: info.Instructions,
		enabled:      info.Enabled,
		connType:     info.Type,
	}
}

// NewBaseConnectorFromConfig is used internally by factory adapters.
func NewBaseConnectorFromConfig(cfg config.Connector) *BaseConnector {
	return NewBaseConnector(BaseConnectorInfo{
		ID:           cfg.ID,
		Name:         cfg.Name,
		Description:  cfg.Description,
		Instructions: cfg.Instructions,
		Enabled:      cfg.Enabled,
		Type:         ConnectorType(cfg.Type),
	})
}

// Getters

func (b *BaseConnector) ID() string           { return b.id }
func (b *BaseConnector) Name() string         { return b.name }
func (b *BaseConnector) Description() string  { return b.description }
func (b *BaseConnector) Instructions() string { return b.instructions }
func (b *BaseConnector) Type() ConnectorType  { return b.connType }
func (b *BaseConnector) Enabled() bool        { return b.enabled }
