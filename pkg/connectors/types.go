package connectors

import (
	"context"
)

// Connector defines the interface for platform integrations
type Connector interface {
	// Metadata
	PlatformName() string
	Version() string

	// Lifecycle
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Health
	HealthCheck() (bool, error)
	Capabilities() []string
}

// BaseConfig is the minimal config all connectors must have
// Platform-specific fields live in their own packages
type BaseConfig struct {
	Platform string `yaml:"platform"` // "telegram", "discord", etc.
	Enabled  bool   `yaml:"enabled"`  // Only load if true
}

// IncomingMessage represents a message from a platform
type IncomingMessage struct {
	MessageID  string            `json:"message_id"`
	PlatformID string            `json:"platform_id"`
	ChatID     string            `json:"chat_id"`
	UserID     string            `json:"user_id"`
	Username   string            `json:"username"`
	Content    string            `json:"content"`
	MediaPaths []string          `json:"media_paths,omitempty"`
	IsGroup    bool              `json:"is_group"`
	ReplyToID  string            `json:"reply_to_id,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Raw        interface{}       `json:"raw,omitempty"`
}

// OutgoingMessage represents a message to send to a platform
type OutgoingMessage struct {
	ChatID    string                 `json:"chat_id"`
	Content   string                 `json:"content"`
	ReplyToID string                 `json:"reply_to_id,omitempty"`
	ParseMode string                 `json:"parse_mode,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
}
