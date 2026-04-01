package vault

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config holds vault configuration
type Config struct {
	Type string `mapstructure:"type" json:"type"` // env, hashicorp, aws, gcp

	// EnvVault config
	Prefix     string `mapstructure:"prefix" json:"prefix,omitempty"`           // Environment variable prefix
	DotEnvPath string `mapstructure:"dotenv_path" json:"dotenv_path,omitempty"` // optional, defaults to ".env"

	// HashiCorp Vault config
	Address   string `mapstructure:"address" json:"address,omitempty"`
	Token     string `mapstructure:"token" json:"-"`
	Namespace string `mapstructure:"namespace" json:"namespace,omitempty"`
	MountPath string `mapstructure:"mount_path" json:"mount_path,omitempty"` // Default: "secret"

	// AWS Secrets Manager config
	AWSRegion    string `mapstructure:"aws_region" json:"aws_region,omitempty"`
	AWSAccessKey string `mapstructure:"aws_access_key" json:"-"`
	AWSSecretKey string `mapstructure:"aws_secret_key" json:"-"`

	// GCP Secret Manager config
	GCPProjectID   string `mapstructure:"gcp_project_id" json:"gcp_project_id,omitempty"`
	GCPCredentials string `mapstructure:"gcp_credentials" json:"-"` // Path to credentials file

	// Caching
	EnableCache  bool          `mapstructure:"enable_cache" json:"enable_cache"`
	CacheTTL     time.Duration `mapstructure:"cache_ttl" json:"cache_ttl,omitempty"`           // Default: 5 minutes
	CacheMaxSize int           `mapstructure:"cache_max_size" json:"cache_max_size,omitempty"` // Default: 1000

	// Security
	AuditLog bool `mapstructure:"audit_log" json:"audit_log"` // Log all secret access
}

// loadVaultConfig reads vault.yaml into a vault.Config.
func LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		return nil, fmt.Errorf("config path empty")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("vault config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	// sensible defaults so vault.yaml can stay minimal
	v.SetDefault("type", "env")
	v.SetDefault("prefix", "ONLYAGENTS_")
	v.SetDefault("enable_cache", true)
	v.SetDefault("audit_log", false)

	v.SetEnvPrefix("ONLYAGENTS")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read vault config: %w", err)
	}

	var vc Config
	if err := v.Unmarshal(&vc); err != nil {
		return nil, fmt.Errorf("unmarshal vault config: %w", err)
	}
	return &vc, nil
}
