package config

import (
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// Config represents the complete agent configuration.
type AgentConfig struct {
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
	LLM              LLMConfig      `mapstructure:"llm"`
	Vault            vault.Config   `mapstructure:"vault"`
	Skills           []SkillBinding `mapstructure:"skills"`
	Channels         []string       `mapstructure:"channels"`
	Soul             SoulConfig     `mapstructure:"soul"`
	User             UserConfig     `mapstructure:"user"`

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

	// MaxToolCallsPerIteration limits tool calls in a single LLM response
	// Default: 3 (if not set or 0)
	// Typical values:
	//   - Conservative (prevent spam): 1-2
	//   - Balanced: 3
	//   - Batch operations: 5-6
	MaxToolCallsPerIteration int `mapstructure:"max_tool_calls_per_iteration"`

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

	// MaxExecutionTime is the overall timeout for agent execution
	// Default: 3m (if not set or 0)
	// Format: "30s", "2m", "5m", etc.
	// Typical values:
	//   - Real-time responses: 30s-1m
	//   - Standard queries: 2m-3m
	//   - Batch processing: 5m-10m
	MaxExecutionTime time.Duration `mapstructure:"max_execution_time"`

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

	// unexported — injected after load, never in yaml
	v vault.Vault
}

type SkillBinding struct {
	Name        string `mapstructure:"name"`
	ConnectorID string `mapstructure:"connector,omitempty"` // empty = use skill default
}

// LLMConfig holds model settings. The actual API key lives in vault.
type LLMConfig struct {
	Provider    string            `mapstructure:"provider"`
	Model       string            `mapstructure:"model"`
	APIKeyVault string            `mapstructure:"api_key_vault"` // vault path, not the key itself
	BaseURL     string            `mapstructure:"base_url"`
	Options     map[string]string `mapstructure:"options"`
}

type SkillConfig struct {
	// Common fields — all skill types
	Name        string `mapstructure:"name"`
	Type        string `mapstructure:"type"` // "cli" | "native"
	Enabled     bool   `mapstructure:"enabled"`
	AccessLevel string `mapstructure:"access_level"`
	Description string `mapstructure:"description"`
	Version     string `mapstructure:"version"`

	Capabilities []string          `mapstructure:"capabilities"`
	Instructions string            `mapstructure:"instructions"`
	Authors      []SkillAuthor     `mapstructure:"authors,omitempty"`
	Homepage     string            `mapstructure:"homepage,omitempty"`
	Requires     SkillRequirements `mapstructure:"requires,omitempty"`
	Security     SkillSecurity     `mapstructure:"security,omitempty"`

	Connector *SkillConnectorSpec `mapstructure:"connector,omitempty"`
	// CLI skill — tools block
	Tools []SkillToolEntry `mapstructure:"tools,omitempty"`

	// Executor config
	Executor ExecutorConfig `mapstructure:"executor,omitempty"`

	// Native skill — arbitrary per-skill config
	RawConfig map[string]any `mapstructure:"config,omitempty"`
}
type SkillConnectorSpec struct {
	Required  bool     `mapstructure:"required"`
	Default   string   `mapstructure:"default"`
	Supported []string `mapstructure:"supported"`
}
type SkillToolEntry struct {
	Name        string           `mapstructure:"name"`
	Description string           `mapstructure:"description"`
	Access      string           `mapstructure:"access"`
	Command     string           `mapstructure:"command"`
	Timeout     int              `mapstructure:"timeout"`
	Parameters  []SkillParamDef  `mapstructure:"parameters"`
	Validation  *SkillValidation `mapstructure:"validation,omitempty"`
}

type SkillParamDef struct {
	Name        string `mapstructure:"name"`
	Type        string `mapstructure:"type"`
	Description string `mapstructure:"description"`
}

type SkillValidation struct {
	AllowedCommands []string `mapstructure:"allowed_commands"`
	DeniedPatterns  []string `mapstructure:"denied_patterns"`
	MaxOutputSize   int      `mapstructure:"max_output_size"`
	RequireConfirm  bool     `mapstructure:"require_confirm"`
}

type BinRequirement struct {
	Name    string            `yaml:"name"`
	Install map[string]string `yaml:"install,omitempty"` // pkg manager → command/url
}

type SkillRequirements struct {
	Bins []BinRequirement `yaml:"bins,omitempty"`
	Env  []string         `yaml:"env,omitempty"`
}

type SkillSecurity struct {
	Sanitized   bool      `mapstructure:"sanitized"`
	SanitizedAt time.Time `mapstructure:"sanitized_at"`
	SanitizedBy string    `mapstructure:"sanitized_by"`
}

type SkillAuthor struct {
	Name  string `mapstructure:"name"`
	Email string `mapstructure:"email,omitempty"`
}

// ExecutorConfig holds CLI executor configuration
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
type ConnectorConfig struct {
	ID           string                    `mapstructure:"id"`
	Name         string                    `mapstructure:"name"`
	Description  string                    `mapstructure:"description"`
	Instructions string                    `mapstructure:"instructions"`
	Type         string                    `mapstructure:"type"`
	Enabled      bool                      `mapstructure:"enabled"`
	VaultPaths   map[string]VaultPathEntry `mapstructure:"vault_paths"`
	RawConfig    map[string]any            `mapstructure:",remain"`
}

// ChannelConfig represents a loaded channel config file
type ChannelConfig struct {
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

type ServerConfig struct {
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

type SoulConfig struct {
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

type UserConfig struct {
	Identity     UserIdentity    `mapstructure:"identity"`
	Background   UserBackground  `mapstructure:"background"`
	DailyRoutine DailyRoutine    `mapstructure:"daily_routine"`
	Preferences  UserPreferences `mapstructure:"preferences"`
}

type UserIdentity struct {
	Name          string `mapstructure:"name"`
	PreferredName string `mapstructure:"preferred_name"`
	Role          string `mapstructure:"role"`
	Timezone      string `mapstructure:"timezone"`
}

type UserBackground struct {
	Professional string `mapstructure:"professional"`
	Personal     string `mapstructure:"personal"`
}

type UserCommunication struct {
	Style              string   `mapstructure:"style"`
	Verbosity          string   `mapstructure:"verbosity"`
	FeedbackPreference string   `mapstructure:"feedback_preference"`
	Preferences        []string `mapstructure:"preferences"`
}

type DailyRoutine struct {
	WorkingHours  string `mapstructure:"working_hours"`
	SleepingHours string `mapstructure:"sleeping_hours"`
}

type UserPreferences struct {
	Technical     []string `mapstructure:"technical"`
	Collaboration []string `mapstructure:"collaboration"`
	WhatIValue    []string `mapstructure:"what_i_value"`
}

// GetVault returns the vault instance attached to this config.
func (c *AgentConfig) GetVault() vault.Vault { return c.v }

// setVault attaches a vault instance (used by loaders only).
func (c *AgentConfig) setVault(v vault.Vault) { c.v = v }

// Close releases vault resources.
func (c *AgentConfig) Close() error {
	if c.v != nil {
		return c.v.Close()
	}
	return nil
}

// Validate checks required fields are present.
func (c *AgentConfig) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("agent.id is required")
	}
	if c.LLM.Provider == "" {
		return fmt.Errorf("llm.provider is required")
	}
	if c.LLM.APIKeyVault == "" {
		return fmt.Errorf("llm.api_key_vault is required (vault path to API key)")
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
