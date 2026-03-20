package config

import (
	"fmt"
	"time"
)

type SkillBinding struct {
	Name        string `mapstructure:"name"`
	ConnectorID string `mapstructure:"connector,omitempty"` // empty = use skill default
}

// LLM holds model settings. The actual API key lives in vault.
type LLM struct {
	Provider   string      `mapstructure:"provider"`
	Model      string      `mapstructure:"model"`
	APIKeyPath string      `mapstructure:"api_key_path"` // vault path, not the key itself
	BaseURL    string      `mapstructure:"base_url"`
	Options    *LLMOptions `mapstructure:"options,omitempty"`
}

type LLMOptions struct {
	MaxTokens    int     `mapstructure:"max_tokens,omitempty"`
	Temperature  float64 `mapstructure:"temperature,omitempty"`
	CacheEnabled bool    `mapstructure:"cache_enabled,omitempty"`
	// CacheKey      string   `mapstructure:"cache_key,omitempty"`
}

// Channel represents a loaded channel config file
type Channel struct {
	Name         string                    `mapstructure:"name"`
	Description  string                    `mapstructure:"description"`
	Instructions string                    `mapstructure:"instructions"`
	Priority     int                       `mapstructure:"priority"`
	Platform     string                    `mapstructure:"platform"`
	Enabled      bool                      `mapstructure:"enabled"`
	VaultPaths   map[string]VaultPathEntry `mapstructure:"vault_paths"`
	RawConfig    map[string]interface{}    `mapstructure:",remain"` // the entire config for platform-specific unmarshaling
}

// VaultPathEntry is shared across channels, connectors, or any resource
// that needs to collect secrets from the user.
type VaultPathEntry struct {
	Path   string `mapstructure:"path"`   // e.g. brave/api_key
	Prompt string `mapstructure:"prompt"` // shown to user
}

type Server struct {
	Host    string `yaml:"host"    mapstructure:"host"`
	Port    int    `yaml:"port"    mapstructure:"port"`
	Version string `yaml:"-"`

	Timeouts TimeoutConfig `yaml:"timeouts" mapstructure:"timeouts"`
	CORS     CORSConfig    `yaml:"cors"     mapstructure:"cors"`
	TLS      TLSConfig     `yaml:"tls"      mapstructure:"tls"`

	VaultPaths map[string]VaultPathEntry `mapstructure:"vault_paths"`
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

type Soul struct {
	Identity     IdentityConfig     `mapstructure:"identity"`
	Behavior     BehaviorConfig     `mapstructure:"behavior"`
	Relationship RelationshipConfig `mapstructure:"relationship"`

	// Extensibility: capture any custom fields user adds
	Custom map[string]interface{} `mapstructure:",remain"`
}

type IdentityConfig struct {
	Role                      string   `mapstructure:"role"`
	DelegationAcknowledgments []string `mapstructure:"delegation_acknowledgments"`
}

type BehaviorConfig struct {
	Communication CommunicationConfig `mapstructure:"communication"`
	Boundaries    []string            `mapstructure:"boundaries"`
	Workflow      string              `mapstructure:"workflow"`
}

type CommunicationConfig struct {
	Style       string   `mapstructure:"style"`
	Preferences []string `mapstructure:"preferences"`
}

type RelationshipConfig struct {
	ToUser string   `mapstructure:"to_user"`
	Values []string `mapstructure:"values"`
}

// Validate checks required fields are present.
func (c *Agent) Validate() error {
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
	if c.MaxToolCallsPerIteration < 0 {
		return fmt.Errorf("max_tool_calls_per_iteration cannot be negative")
	}
	if c.MaxCumulativeToolCalls < 0 {
		return fmt.Errorf("max_cumulative_tool_calls cannot be negative")
	}
	if c.MaxToolResultTokens < 0 {
		return fmt.Errorf("max_tool_result_tokens cannot be negative")
	}
	if c.MaxExecutionTime < 0 {
		return fmt.Errorf("max_execution_time cannot be negative")
	}
	if c.SimilarCallThreshold < 0 {
		return fmt.Errorf("similar_call_threshold cannot be negative")
	}
	return nil
}
