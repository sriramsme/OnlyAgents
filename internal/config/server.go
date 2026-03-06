package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// LoadServerConfig reads configs/server.yaml.
func LoadServerConfig() (*ServerConfig, error) {
	cfg, err := loadServer()
	if err != nil {
		return nil, fmt.Errorf("load server config: %w", err)
	}
	return cfg, nil
}

// loadServer reads a server config file into a ServerConfig struct.
func loadServer() (*ServerConfig, error) {
	configPath := ServerConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("server config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	setServerDefaults(v)
	v.SetEnvPrefix("ONLYAGENTS")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read server config: %w", err)
	}

	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal server config: %w", err)
	}
	return &cfg, nil
}

func setServerDefaults(v *viper.Viper) {
	v.SetDefault("host", "")
	v.SetDefault("port", 8080)
	v.SetDefault("read_timeout", 30*time.Second)
	v.SetDefault("write_timeout", 120*time.Second)
	v.SetDefault("idle_timeout", 60*time.Second)
	v.SetDefault("version", "0.1.0")
}
