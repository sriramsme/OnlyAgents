package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// LoadServerConfig reads configs/server.yaml.
func LoadServerConfig(configPath string) (*ServerConfig, error) {
	cfg, err := loadServer(configPath)
	if err != nil {
		return nil, fmt.Errorf("load server config: %w", err)
	}
	return cfg, nil
}

// loadServer reads a server config file into a ServerConfig struct.
func loadServer(configPath string) (*ServerConfig, error) {
	v := viper.New()
	setServerDefaults(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("server")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.onlyagents")
		v.AddConfigPath("/etc/onlyagents")
	}

	v.SetEnvPrefix("ONLYAGENTS")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read server config: %w", err)
		}
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
