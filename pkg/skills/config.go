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
	Name        string `mapstructure:"name"         json:"name"`
	Type        string `mapstructure:"type"         json:"type"` // "cli" | "native"
	Enabled     bool   `mapstructure:"enabled"      json:"enabled"`
	AccessLevel string `mapstructure:"access_level" json:"access_level"`
	Description string `mapstructure:"description"  json:"description,omitempty"`
	Version     string `mapstructure:"version"      json:"version,omitempty"`

	Capabilities []string     `mapstructure:"capabilities"    json:"capabilities,omitempty"`
	Instructions string       `mapstructure:"instructions"    json:"instructions,omitempty"`
	Authors      []Author     `mapstructure:"authors"         json:"authors,omitempty"`
	Homepage     string       `mapstructure:"homepage"        json:"homepage,omitempty"`
	Requires     Requirements `mapstructure:"requires"        json:"requires"`

	Connector *ConnectorSpec `mapstructure:"connector" json:"connector,omitempty"`

	Groups map[string]string `mapstructure:"groups" json:"groups,omitempty"`
	Tools  []ToolEntry       `mapstructure:"tools"  json:"tools,omitempty"`

	Executor ExecutorConfig `mapstructure:"executor" json:"executor"`

	LLM *llm.Config `mapstructure:"llm" json:"llm,omitempty"`

	Security config.SecurityConfig `mapstructure:"security" json:"security"`
	Config   map[string]any        `mapstructure:"config"   json:"config,omitempty"`
}

// ExecDef describes a structured external command — binary name plus args.
// No shell is involved. The LLM only supplies parameter values that fill
// {{param}} placeholders in Args; the binary itself is always config-defined.
type ExecDef struct {
	Command    string   `mapstructure:"command"`               // binary name: "mkdir", "cat", "rg"
	Args       []string `mapstructure:"args"`                  // may contain {{param}} placeholders
	StdinParam string   `mapstructure:"stdin_param,omitempty"` // param whose value is piped to stdin
}

type ToolEntry struct {
	Name        string      `mapstructure:"name"                json:"name"`
	Description string      `mapstructure:"description"         json:"description,omitempty"`
	Group       string      `mapstructure:"group"               json:"group,omitempty"`
	Access      string      `mapstructure:"access"              json:"access"`
	Exec        ExecDef     `mapstructure:"exec"                json:"exec"`
	Timeout     int         `mapstructure:"timeout"             json:"timeout,omitempty"`
	Parameters  []ParamDef  `mapstructure:"parameters"          json:"parameters,omitempty"`
	Validation  *Validation `mapstructure:"validation"          json:"validation,omitempty"`
}

type ParamDef struct {
	Name        string `mapstructure:"name"                json:"name"`
	Type        string `mapstructure:"type"                json:"type"`
	Description string `mapstructure:"description"         json:"description,omitempty"`
}

// Validation is now minimal. AllowedCommands and DeniedPatterns are gone —
// the binary is fixed in config, not user-supplied, so there is no shell
// string to match against. RequireConfirm remains for destructive tools.
type Validation struct {
	MaxOutputSize  int  `mapstructure:"max_output_size"  json:"max_output_size,omitempty"`
	RequireConfirm bool `mapstructure:"require_confirm"  json:"require_confirm"`
}

// ExecutorConfig holds per-skill execution constraints.
// Isolation boundary (workdir, network, bins) is owned by root SecurityConfig.
type ExecutorConfig struct {
	MaxOutputSize      int    `mapstructure:"max_output_size"`      // bytes; 0 = unlimited
	MaxExecutionTime   int    `mapstructure:"max_execution_time"`   // seconds; 0 = 60s default
	MissingBinBehavior string `mapstructure:"missing_bin_behavior"` // error | warn | disable
}

type ConnectorSpec struct {
	Required  bool     `mapstructure:"required"   json:"required"`
	Default   string   `mapstructure:"default"    json:"default,omitempty"`
	Supported []string `mapstructure:"supported"  json:"supported,omitempty"`
}

type BinRequirement struct {
	Name    string            `mapstructure:"name"    json:"name"`
	Install map[string]string `mapstructure:"install" json:"install,omitempty"`
}

type Requirements struct {
	Bins []BinRequirement `mapstructure:"bins" json:"bins,omitempty"`
	Env  []string         `mapstructure:"env"  json:"env,omitempty"`
}

type Author struct {
	Name  string `mapstructure:"name"  json:"name"`
	Email string `mapstructure:"email" json:"email,omitempty"`
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
