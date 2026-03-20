package skills

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/internal/paths"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

type Config struct {
	// Common fields — all skill types
	Name        string `mapstructure:"name"`
	Type        string `mapstructure:"type"` // "cli" | "native"
	Enabled     bool   `mapstructure:"enabled"`
	AccessLevel string `mapstructure:"access_level"`
	Description string `mapstructure:"description"`
	Version     string `mapstructure:"version"`

	Capabilities []string     `mapstructure:"capabilities"`
	Instructions string       `mapstructure:"instructions"`
	Authors      []Author     `mapstructure:"authors,omitempty"`
	Homepage     string       `mapstructure:"homepage,omitempty"`
	Requires     Requirements `mapstructure:"requires,omitempty"`

	Connector *ConnectorSpec `mapstructure:"connector,omitempty"`

	// CLI skill — tools block

	Groups map[string]string `mapstructure:"groups,omitempty"`
	Tools  []ToolEntry       `mapstructure:"tools,omitempty"`

	// Executor config
	Executor ExecutorConfig `mapstructure:"executor,omitempty"`

	// Optional LLM configuration
	LLM *llm.Config `mapstructure:"llm,omitempty"`

	Security config.SecurityConfig `mapstructure:"security,omitempty"`
	// For skill-specific extensions
	Config map[string]any `mapstructure:"config,omitempty"`
}

type ConnectorSpec struct {
	Required  bool     `mapstructure:"required"`
	Default   string   `mapstructure:"default"`
	Supported []string `mapstructure:"supported"`
}
type ToolEntry struct {
	Name        string      `mapstructure:"name"`
	Description string      `mapstructure:"description"`
	Access      string      `mapstructure:"access"`
	Command     string      `mapstructure:"command"`
	Timeout     int         `mapstructure:"timeout"`
	Parameters  []ParamDef  `mapstructure:"parameters"`
	Validation  *Validation `mapstructure:"validation,omitempty"`
	Group       string      `mapstructure:"group,omitempty"`
}

type ParamDef struct {
	Name        string `mapstructure:"name"`
	Type        string `mapstructure:"type"`
	Description string `mapstructure:"description"`
}

type Validation struct {
	AllowedCommands []string `mapstructure:"allowed_commands"`
	DeniedPatterns  []string `mapstructure:"denied_patterns"`
	MaxOutputSize   int      `mapstructure:"max_output_size"`
	RequireConfirm  bool     `mapstructure:"require_confirm"`
}

type BinRequirement struct {
	Name    string            `yaml:"name"`
	Install map[string]string `yaml:"install,omitempty"` // pkg manager → command/url
}

type Requirements struct {
	Bins []BinRequirement `yaml:"bins,omitempty"`
	Env  []string         `yaml:"env,omitempty"`
}

type Author struct {
	Name  string `mapstructure:"name"`
	Email string `mapstructure:"email,omitempty"`
}

// holds CLI executor configuration
type ExecutorConfig struct {
	// Security settings
	AllowedShells      []string `mapstructure:"allowed_shells"`       // Default: ["bash", "sh"]
	MaxOutputSize      int      `mapstructure:"max_output_size"`      // Bytes, default: 1MB
	MaxExecutionTime   int      `mapstructure:"max_execution_time"`   // Seconds, default: 60
	MissingBinBehavior string   `mapstructure:"missing_bin_behavior"` // Default: "error  warn | error | disable"

	// Sandboxing (future)
	UseSandbox  bool   `mapstructure:"use_sandbox"`
	SandboxType string `mapstructure:"sandbox_type"` // docker, firejail, etc.
}

// LoadAllConfigs loads all *.yaml files from the skills config dir.
func LoadAllConfigs(dir string) (map[string]*Config, error) {
	if dir == "" {
		dir = paths.SkillsDir()
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read skill dir: %w", err)
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

		configs[cfg.Name] = cfg
	}

	return configs, nil
}

func LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		return nil, fmt.Errorf("config path empty")
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("skill config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg, func(dc *mapstructure.DecoderConfig) {
		dc.TagName = "mapstructure"
		dc.WeaklyTypedInput = true
		dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		)
	}); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.Config = v.AllSettings()
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Common
	v.SetDefault("enabled", true)
	v.SetDefault("version", "1.0.0")
	v.SetDefault("access_level", "read")

	// Executor
	v.SetDefault("executor.allowed_shells", []string{"bash", "sh"})
	v.SetDefault("executor.max_output_size", 1024*1024) // 1MB
	v.SetDefault("executor.max_execution_time", 60)     // seconds
	v.SetDefault("executor.use_sandbox", false)
}
