package agents

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"github.com/sriramsme/OnlyAgents/internal/paths"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

// Config represents the complete agent configuration.
type Config struct {
	ID               string         `mapstructure:"id"`
	Name             string         `mapstructure:"name"`
	Description      string         `mapstructure:"description"`
	IsExecutive      bool           `mapstructure:"is_executive"`
	IsGeneral        bool           `mapstructure:"is_general"`
	Enabled          bool           `mapstructure:"enabled"`
	Role             string         `mapstructure:"role"`
	StreamingEnabled bool           `mapstructure:"streaming_enabled"`
	MaxConcurrency   int            `mapstructure:"max_concurrency"`
	BufferSize       int            `mapstructure:"buffer_size"`
	LLM              llm.Config     `mapstructure:"llm"`
	Skills           []SkillBinding `mapstructure:"skills"`
	Channels         []string       `mapstructure:"channels"`
	Soul             Soul           `mapstructure:"soul"`

	// ============================================
	// EXECUTION LIMITS (Guard Rails)
	// ============================================
	// These fields control agent execution to prevent runaway loops,
	// excessive costs, and performance issues.

	// MaxIterations is the maximum number of LLM request/response cycles
	// Default: 10 (if not set or 0)
	// Typical values:
	//   - Simple agents (email, calculator): 5
	//   - Standard agents (researcher): 10
	//   - Complex agents (multi-step workflows): 15
	MaxIterations int `mapstructure:"max_iterations"`

	// MaxCumulativeToolCalls is the total tool calls across all iterations
	// Default: 15 (if not set or 0)
	// Prevents infinite loops and controls costs
	// Typical values:
	//   - Simple agents: 5-8
	//   - Standard agents: 15
	//   - Complex agents: 25-30
	MaxCumulativeToolCalls int `mapstructure:"max_cumulative_tool_calls"`

	// MaxToolResultTokens truncates individual tool results to this size
	// Default: 2000 (if not set or 0)
	// Prevents context explosion from large tool outputs
	// Typical values:
	//   - Summary-focused: 1000
	//   - Balanced: 2000
	//   - Document processing: 4000
	MaxToolResultTokens int `mapstructure:"max_tool_result_tokens"`

	// EnableEarlyStopping detects and stops repeated tool calls
	// Default: true (if not explicitly set to false)
	// CRITICAL for search-heavy agents (prevents search spam)
	// Set to false only for legitimate repeated operations
	EnableEarlyStopping *bool `mapstructure:"enable_early_stopping"`

	// SimilarCallThreshold is how many similar calls before early stopping
	// Default: 3 (if not set or 0)
	// Only applies if EnableEarlyStopping is true
	// Typical values:
	//   - Aggressive (prevent loops): 2
	//   - Balanced: 3
	//   - Forgiving (allow exploration): 4-5
	SimilarCallThreshold int `mapstructure:"similar_call_threshold"`
}

type SkillBinding struct {
	Name        string `mapstructure:"name"`
	ConnectorID string `mapstructure:"connector,omitempty"` // empty = use skill default
}

// load reads an agent config file into a Config struct.
// It does not validate or attach a vault — callers do that.
func LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		return nil, fmt.Errorf("config path empty")
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("agent config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	setDefaults(v)
	v.SetEnvPrefix("ONLYAGENTS")
	v.AutomaticEnv()

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

	return &cfg, nil
}

// LoadAllAgentsConfig loads every *.yaml under dir, sharing a single vault
// instance across all of them. Returns the configs and the vault so the
// caller owns its lifecycle.
func LoadAllConfigs(dir string) ([]*Config, error) {
	if dir == "" {
		dir = paths.AgentsDir()
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no agents found in %s", dir)
	}
	var configs []*Config
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}

		cfg, err := LoadConfig(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", f.Name(), err)
		}

		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("config validation %s: %w", f.Name(), err)
		}
		if cfg.Enabled {
			configs = append(configs, cfg)
		}
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("no agents loaded. Make sure at least the executive and general  agents are enabled")
	}
	return configs, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("agent.max_concurrency", 10)
	v.SetDefault("agent.buffer_size", 100)
	v.SetDefault("agent.streaming_enabled", true)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")
	v.SetDefault("llm.provider", "anthropic")
	v.SetDefault("llm.model", "claude-sonnet-4-20250514")
	v.SetDefault("llm.options.max_tokens", 0)
	v.SetDefault("llm.options.temperature", 1.0)
	v.SetDefault("vault.type", "env")
	v.SetDefault("vault.prefix", "ONLYAGENTS_")
	v.SetDefault("vault.enable_cache", true)
	v.SetDefault("vault.audit_log", false)
	v.SetDefault("max_iterations", 10)
	v.SetDefault("max_cumulative_tool_calls", 15)
	v.SetDefault("max_tool_result_tokens", 1500)
	v.SetDefault("enable_early_stopping", true)
	v.SetDefault("similar_call_threshold", 3)
}

// Validate checks required fields are present.
func (c *Config) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("agent.id is required")
	}
	if c.LLM.Provider == "" {
		return fmt.Errorf("llm.provider is required")
	}
	if c.LLM.APIKeyPath == "" {
		return fmt.Errorf("llm.api_key_path is required (vault path to API key)")
	}
	if c.Name == "" {
		return fmt.Errorf("agent name is required")
	}

	// Validate execution limits (optional - defaults will be applied)
	if c.MaxIterations < 0 {
		return fmt.Errorf("max_iterations cannot be negative")
	}
	if c.MaxCumulativeToolCalls < 0 {
		return fmt.Errorf("max_cumulative_tool_calls cannot be negative")
	}
	if c.MaxToolResultTokens < 0 {
		return fmt.Errorf("max_tool_result_tokens cannot be negative")
	}
	if c.SimilarCallThreshold < 0 {
		return fmt.Errorf("similar_call_threshold cannot be negative")
	}
	return nil
}
