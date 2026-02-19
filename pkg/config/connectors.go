package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ConnectorConfigFile represents a loaded connector config file
type ConnectorConfigFile struct {
	Name      string                 // derived from filename
	Platform  string                 `mapstructure:"platform"`
	Enabled   bool                   `mapstructure:"enabled"`
	RawConfig map[string]interface{} // the entire config for platform-specific unmarshaling
}

// LoadConnectorConfig loads a single connector config file
func LoadConnectorConfig(configPath string) (*ConnectorConfigFile, error) {
	v := viper.New()
	v.SetConfigFile(configPath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg ConnectorConfigFile
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Store raw config for platform-specific unmarshaling
	cfg.RawConfig = v.AllSettings()

	// Extract name from filename (without extension)
	cfg.Name = filepath.Base(configPath)
	cfg.Name = cfg.Name[:len(cfg.Name)-len(filepath.Ext(cfg.Name))]

	if cfg.Platform == "" {
		return nil, fmt.Errorf("platform field is required")
	}

	return &cfg, nil
}

// LoadAllConnectorConfigs loads all connector configs from a directory
func LoadAllConnectorConfigs(dir string) (map[string]*ConnectorConfigFile, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read connectors dir: %w", err)
	}

	configs := make(map[string]*ConnectorConfigFile)

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}

		cfg, err := LoadConnectorConfig(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", f.Name(), err)
		}

		configs[cfg.Name] = cfg
	}

	return configs, nil
}
