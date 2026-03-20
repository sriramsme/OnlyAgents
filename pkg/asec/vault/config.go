package vault

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config holds vault configuration
type Config struct {
	Type string `mapstructure:"type"` // env, hashicorp, aws, gcp

	// EnvVault config
	Prefix     string `mapstructure:"prefix"`      // Environment variable prefix
	DotEnvPath string `mapstructure:"dotenv_path"` // optional, defaults to ".env"

	// HashiCorp Vault config
	Address   string `mapstructure:"address"`
	Token     string `mapstructure:"token"`
	Namespace string `mapstructure:"namespace"`
	MountPath string `mapstructure:"mount_path"` // Default: "secret"

	// AWS Secrets Manager config
	AWSRegion    string `mapstructure:"aws_region"`
	AWSAccessKey string `mapstructure:"aws_access_key"`
	AWSSecretKey string `mapstructure:"aws_secret_key"`

	// GCP Secret Manager config
	GCPProjectID   string `mapstructure:"gcp_project_id"`
	GCPCredentials string `mapstructure:"gcp_credentials"` // Path to credentials file

	// Caching
	EnableCache  bool          `mapstructure:"enable_cache"`
	CacheTTL     time.Duration `mapstructure:"cache_ttl"`      // Default: 5 minutes
	CacheMaxSize int           `mapstructure:"cache_max_size"` // Default: 1000

	// Security
	AuditLog bool `mapstructure:"audit_log"` // Log all secret access
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
