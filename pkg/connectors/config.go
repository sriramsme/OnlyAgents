package connectors

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"

	"github.com/sriramsme/OnlyAgents/internal/paths"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

type Config struct {
	ID           string                     `mapstructure:"id"`
	Name         string                     `mapstructure:"name"`
	Description  string                     `mapstructure:"description"`
	Instructions string                     `mapstructure:"instructions"`
	Type         string                     `mapstructure:"type"`
	Enabled      bool                       `mapstructure:"enabled"`
	VaultPaths   map[string]vault.PathEntry `mapstructure:"vault_paths"`
	RawConfig    map[string]any             `mapstructure:",remain"`
}

// LoadConnectorConfig loads a single connector config file
func LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		return nil, fmt.Errorf("config path empty")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("connector config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Store raw config for platform-specific unmarshaling
	cfg.RawConfig = v.AllSettings()

	if cfg.ID == "" {
		return nil, fmt.Errorf("id field is required")
	}

	return &cfg, nil
}

// LoadAllConnectorConfigs loads all connector configs from a directory
func LoadAllConfigs(dir string) (map[string]*Config, error) {
	if dir == "" {
		dir = paths.ConnectorsDir()
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

		configs[cfg.ID] = cfg
	}

	return configs, nil
}
