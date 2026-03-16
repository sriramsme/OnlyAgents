package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type User struct {
	Identity     UserIdentity    `mapstructure:"identity"`
	Background   UserBackground  `mapstructure:"background"`
	DailyRoutine DailyRoutine    `mapstructure:"daily_routine"`
	Preferences  UserPreferences `mapstructure:"preferences"`
}

type UserIdentity struct {
	Name          string `mapstructure:"name"`
	PreferredName string `mapstructure:"preferred_name"`
	Role          string `mapstructure:"role"`
	Timezone      string `mapstructure:"timezone"`
}

type UserBackground struct {
	Professional string `mapstructure:"professional"`
	Personal     string `mapstructure:"personal"`
}

type UserCommunication struct {
	Style              string   `mapstructure:"style"`
	Verbosity          string   `mapstructure:"verbosity"`
	FeedbackPreference string   `mapstructure:"feedback_preference"`
	Preferences        []string `mapstructure:"preferences"`
}

type DailyRoutine struct {
	WorkingHours  string `mapstructure:"working_hours"`
	SleepingHours string `mapstructure:"sleeping_hours"`
}

type UserPreferences struct {
	Technical     []string `mapstructure:"technical"`
	Collaboration []string `mapstructure:"collaboration"`
	WhatIValue    []string `mapstructure:"what_i_value"`
}

// LoadUserConfig loads the user profile
func LoadUserConfig() (*User, error) {
	configPath := UserConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("user config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read user config: %w", err)
	}

	var cfg User
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal user config: %w", err)
	}

	return &cfg, nil
}

func SaveUserConfig(cfg *User) error {
	v := viper.New()
	path := UserConfigPath()
	v.SetConfigFile(path)

	// Marshal the config to map for viper
	data := map[string]interface{}{
		"identity":      cfg.Identity,
		"background":    cfg.Background,
		"preferences":   cfg.Preferences,
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
