package telegram

import "github.com/sriramsme/OnlyAgents/internal/config"

// Config holds Telegram-specific configuration
type Config struct {
	config.ChannelConfig `mapstructure:",squash"`
	Mode                 string `mapstructure:"mode"` // "polling" or "webhook"

	// Routing
	DefaultAgent string `mapstructure:"default_agent"` // Usually "executive"

	// Security
	AllowFrom []string `mapstructure:"allow_from,omitempty"`
	Proxy     string   `mapstructure:"proxy,omitempty"`

	// Mode-specific
	Webhook *WebhookConfig `mapstructure:"webhook,omitempty"`
	Polling *PollingConfig `mapstructure:"polling,omitempty"`
}

type WebhookConfig struct {
	URL                string `mapstructure:"url"`
	ListenAddr         string `mapstructure:"listen_addr"`
	Path               string `mapstructure:"path"`
	DropPendingUpdates bool   `mapstructure:"drop_pending_updates"`
	MaxConnections     int    `mapstructure:"max_connections"`
}

// PollingConfig holds polling settings
type PollingConfig struct {
	Timeout    int `mapstructure:"timeout"`
	Limit      int `mapstructure:"limit"`
	RetryDelay int `mapstructure:"retry_delay"`
	MaxRetries int `mapstructure:"max_retries"`
}
