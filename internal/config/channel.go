package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// LoadConnectorConfig loads a single connector config file
func loadChannelConfig(configPath string) (*Channel, error) {
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

	var cfg Channel
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

// LoadAllConnectorConfigs loads all connector configs from a directory
func LoadAllChannelConfigs() (map[string]*Channel, error) {
	dir := ChannelsDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read connectors dir: %w", err)
	}

	configs := make(map[string]*Channel)

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}

		cfg, err := loadChannelConfig(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", f.Name(), err)
		}

		configs[cfg.Platform] = cfg
	}

	return configs, nil
}
