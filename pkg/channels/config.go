package channels

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/sriramsme/OnlyAgents/internal/paths"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// Channel represents a loaded channel config file
type Config struct {
	Name         string                     `mapstructure:"name"`
	Description  string                     `mapstructure:"description"`
	Instructions string                     `mapstructure:"instructions"`
	Priority     int                        `mapstructure:"priority"`
	Platform     string                     `mapstructure:"platform"`
	Enabled      bool                       `mapstructure:"enabled"`
	VaultPaths   map[string]vault.PathEntry `mapstructure:"vault_paths"`
	AllowFrom    []string                   `mapstructure:"allow_from,omitempty"`

	// Config is the entire config for platform-specific unmarshaling
	Config map[string]interface{} `mapstructure:",remain"` // the entire config for platform-specific unmarshaling

	// defaultAgent string
}

// LoadConnectorConfig loads a single connector config file
func LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		return nil, fmt.Errorf("config path empty")
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("channel config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg, func(dc *mapstructure.DecoderConfig) {
		dc.TagName = "mapstructure"
		dc.WeaklyTypedInput = true
		dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			mapstructure.TextUnmarshallerHookFunc(),
		)
	}); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.Platform == "" {
		return nil, fmt.Errorf("platform field is required")
	}
	return &cfg, nil
}

// LoadAllConfigs loads all channel configs from a directory
func LoadAllConfigs(dir string) (map[string]*Config, error) {
	if dir == "" {
		dir = paths.ChannelsDir()
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read connectors dir: %w", err)
	}

	configs := make(map[string]*Config)

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}

		cfg, err := LoadConfig(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", f.Name(), err)
		}

		configs[cfg.Platform] = cfg
	}

	return configs, nil
}
