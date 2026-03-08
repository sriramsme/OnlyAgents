package channels

import (
	"context"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// Connector defines the interface for platform integrations
type Channel interface {
	// Metadata
	PlatformName() string
	Version() string

	// Lifecycle
	Connect() error
	Disconnect() error
	Start() error
	Stop() error

	Send(ctx context.Context, msg OutgoingMessage) error

	// Health
	HealthCheck() (bool, error)
}

type Registry struct {
	mu       sync.RWMutex
	channels map[string]Channel

	active Channel
}

type TokenStreamer interface {
	SendToken(ctx context.Context, channel *core.ChannelMetadata, token, accumulated string) error
}

// BaseConfig is the minimal config all connectors must have
// Platform-specific fields live in their own packages
type Config struct {
	Platform string `yaml:"platform"` // "telegram", "discord", etc.
	Enabled  bool   `yaml:"enabled"`  // Only load if true

	// Routing
	DefaultAgent string `yaml:"default_agent"` // Usually "executive"

	// Security
	AllowFrom []string `yaml:"allow_from,omitempty"`
}

// IncomingMessage represents a message from a platform
type IncomingMessage struct {
	MessageID  string                `json:"message_id"`
	PlatformID string                `json:"platform_id"`
	Channel    *core.ChannelMetadata `json:"channel"`
	Content    string                `json:"content"`
	MediaPaths []string              `json:"media_paths,omitempty"`
	IsGroup    bool                  `json:"is_group"`
	ReplyToID  string                `json:"reply_to_id,omitempty"`
	Metadata   map[string]string     `json:"metadata,omitempty"`
	Raw        interface{}           `json:"raw,omitempty"`
}

// OutgoingMessage represents a message to send to a platform
type OutgoingMessage struct {
	Channel   *core.ChannelMetadata  `json:"channel"`
	Content   string                 `json:"content"`
	ReplyToID string                 `json:"reply_to_id,omitempty"`
	ParseMode string                 `json:"parse_mode,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
}
