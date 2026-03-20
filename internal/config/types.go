package config

import (
	"time"
)

// Channel represents a loaded channel config file
type Channel struct {
	Name         string                    `mapstructure:"name"`
	Description  string                    `mapstructure:"description"`
	Instructions string                    `mapstructure:"instructions"`
	Priority     int                       `mapstructure:"priority"`
	Platform     string                    `mapstructure:"platform"`
	Enabled      bool                      `mapstructure:"enabled"`
	VaultPaths   map[string]VaultPathEntry `mapstructure:"vault_paths"`
	RawConfig    map[string]interface{}    `mapstructure:",remain"` // the entire config for platform-specific unmarshaling
}

// VaultPathEntry is shared across channels, connectors, or any resource
// that needs to collect secrets from the user.
type VaultPathEntry struct {
	Path   string `mapstructure:"path"`   // e.g. brave/api_key
	Prompt string `mapstructure:"prompt"` // shown to user
}

type Server struct {
	Host    string `yaml:"host"    mapstructure:"host"`
	Port    int    `yaml:"port"    mapstructure:"port"`
	Version string `yaml:"-"`

	Timeouts TimeoutConfig `yaml:"timeouts" mapstructure:"timeouts"`
	CORS     CORSConfig    `yaml:"cors"     mapstructure:"cors"`
	TLS      TLSConfig     `yaml:"tls"      mapstructure:"tls"`

	VaultPaths map[string]VaultPathEntry `mapstructure:"vault_paths"`
}

type TimeoutConfig struct {
	Read     time.Duration `yaml:"read"     mapstructure:"read"`
	Write    time.Duration `yaml:"write"    mapstructure:"write"`
	Idle     time.Duration `yaml:"idle"     mapstructure:"idle"`
	Shutdown time.Duration `yaml:"shutdown" mapstructure:"shutdown"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins" mapstructure:"allowed_origins"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"   mapstructure:"enabled"`
	CertPath string `yaml:"cert_path" mapstructure:"cert_path"`
	KeyPath  string `yaml:"key_path"  mapstructure:"key_path"`
}
