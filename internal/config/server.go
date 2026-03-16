package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// LoadServerConfig reads configs/server.yaml.
func LoadServerConfig() (*Server, error) {
	cfg, err := loadServer()
	if err != nil {
		return nil, fmt.Errorf("load server config: %w", err)
	}
	return cfg, nil
}

// loadServer reads a server config file into a Server struct.
func loadServer() (*Server, error) {
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

	var cfg Server
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal server config: %w", err)
	}
	return &cfg, nil
}

func setServerDefaults(v *viper.Viper) {
	v.SetDefault("host", "0.0.0.0")
	v.SetDefault("port", 19965)
	v.SetDefault("timeouts.read", "30s")
	v.SetDefault("timeouts.write", "30s")
	v.SetDefault("timeouts.idle", "120s")
	v.SetDefault("timeouts.shutdown", "10s")
	v.SetDefault("cors.allowed_origins", []string{
		"http://localhost:5173",
		"http://localhost:19965",
	})
	v.SetDefault("tls.enabled", false)
}
