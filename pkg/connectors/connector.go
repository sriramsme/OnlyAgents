package connectors

// Connector interface for platform integrations
type Connector interface {
	PlatformName() string
	Version() string

	Connect(credentials map[string]string) error
	Disconnect() error
	HealthCheck() (bool, error)

	Capabilities() []string
}
