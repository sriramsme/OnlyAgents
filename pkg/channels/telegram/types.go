package telegram

// Config holds Telegram-specific configuration
type Config struct {
	// Base fields (every connector has these)
	Platform string `yaml:"platform"` // "telegram"
	Enabled  bool   `yaml:"enabled"`

	// Telegram-specific fields
	Credentials Credentials `yaml:"credentials"`
	Mode        string      `yaml:"mode"` // "polling" or "webhook"

	// Routing
	DefaultAgent string `yaml:"default_agent"` // Usually "executive"

	// Security
	AllowFrom []string `yaml:"allow_from,omitempty"`
	Proxy     string   `yaml:"proxy,omitempty"`

	// Mode-specific
	Webhook *WebhookConfig `yaml:"webhook,omitempty"`
	Polling *PollingConfig `yaml:"polling,omitempty"`
}

// Credentials holds vault references
type Credentials struct {
	BotToken string `yaml:"bot_token"` // Vault key name
}

type WebhookConfig struct {
	URL                string `yaml:"url"`
	ListenAddr         string `yaml:"listen_addr"`
	Path               string `yaml:"path"`
	DropPendingUpdates bool   `yaml:"drop_pending_updates"`
	MaxConnections     int    `yaml:"max_connections"`
}

// PollingConfig holds polling settings
type PollingConfig struct {
	Timeout    int `yaml:"timeout"`
	Limit      int `yaml:"limit"`
	RetryDelay int `yaml:"retry_delay"`
	MaxRetries int `yaml:"max_retries"`
}
