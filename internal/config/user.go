package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// LoadUserConfig loads the user profile
func LoadUserConfig() (*UserConfig, error) {
	configPath := UserConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("user config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read user config: %w", err)
	}

	var cfg UserConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal user config: %w", err)
	}

	return &cfg, nil
}

func SaveUserConfig(cfg *UserConfig) error {
	v := viper.New()
	path := UserConfigPath()
	v.SetConfigFile(path)

	// Marshal the config to map for viper
	data := map[string]interface{}{
		"identity":      cfg.Identity,
		"background":    cfg.Background,
		"work":          cfg.Work,
		"preferences":   cfg.Preferences,
		"learned":       cfg.Learned,
		"daily_routine": cfg.DailyRoutine,
	}

	for key, val := range data {
		v.Set(key, val)
	}

	if err := v.WriteConfig(); err != nil {
		// If file doesn't exist, create it
		if os.IsNotExist(err) {
			return v.SafeWriteConfig()
		}
		return fmt.Errorf("write soul config: %w", err)
	}

	return nil
	// ...
}
