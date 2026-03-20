package api

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"

	"github.com/sriramsme/OnlyAgents/internal/paths"
)

type Config struct {
	Host    string `yaml:"host"    mapstructure:"host"`
	Port    int    `yaml:"port"    mapstructure:"port"`
	Version string `yaml:"-"`

	Timeouts TimeoutConfig `yaml:"timeouts" mapstructure:"timeouts"`
	CORS     CORSConfig    `yaml:"cors"     mapstructure:"cors"`
	TLS      TLSConfig     `yaml:"tls"      mapstructure:"tls"`

	VaultPaths map[string]vault.PathEntry `mapstructure:"vault_paths"`
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

// LoadServerConfig reads configs/server.yaml.
func LoadConfig() (*Config, error) {
	cfg, err := loadServer()
	if err != nil {
		return nil, fmt.Errorf("load server config: %w", err)
	}
	return cfg, nil
}

// loadServer reads a server config file into a Server struct.
func loadServer() (*Config, error) {
	configPath := paths.ServerConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("server config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	setDefaults(v)
	v.SetEnvPrefix("ONLYAGENTS")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read server config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal server config: %w", err)
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
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
