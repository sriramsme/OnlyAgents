package config

import (
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// Config represents the complete agent configuration.
type Config struct {
	ID             string           `mapstructure:"id"`
	Name           string           `mapstructure:"name"`
	IsExecutive    bool             `mapstructure:"is_executive"`
	IsGeneral      bool             `mapstructure:"is_general"`
	Role           string           `mapstructure:"role"`
	UserRef        string           `mapstructure:"user_ref"`
	MaxConcurrency int              `mapstructure:"max_concurrency"`
	BufferSize     int              `mapstructure:"buffer_size"`
	Logging        LoggingConfig    `mapstructure:"logging"`
	Security       SecurityConfig   `mapstructure:"security"`
	LLM            LLMConfig        `mapstructure:"llm"`
	Vault          vault.Config     `mapstructure:"vault"`
	Skills         []string         `mapstructure:"skills"`
	Platforms      []PlatformConfig `mapstructure:"platforms"`
	Connectors     []string         `mapstructure:"connectors"`
	Channels       []string         `mapstructure:"channels"`
	Soul           SoulConfig       `mapstructure:"soul"`
	User           UserConfig       `mapstructure:"user"`

	// unexported — injected after load, never in yaml
	v vault.Vault
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type SecurityConfig struct {
	KeyPath         string `mapstructure:"key_path"`
	CredentialsPath string `mapstructure:"credentials_path"`
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
	Name    string         `mapstructure:"name"`
	Enabled bool           `mapstructure:"enabled"`
	Config  map[string]any `mapstructure:"config"`
}

type PlatformConfig struct {
	Name        string         `mapstructure:"name"`
	Type        string         `mapstructure:"type"`
	Enabled     bool           `mapstructure:"enabled"`
	Credentials map[string]any `mapstructure:"credentials"` // vault paths
}

type ConnectorConfig struct {
	Name        string         `mapstructure:"name"`
	Type        string         `mapstructure:"type"`
	Enabled     bool           `mapstructure:"enabled"`
	Credentials map[string]any `mapstructure:"credentials"` // vault paths
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	APIKeyVault  string        `mapstructure:"api_key_vault"` // empty = no auth (local dev)
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
	Version      string        `mapstructure:"version"`
}

type SoulConfig struct {
	Identity     IdentityConfig     `mapstructure:"identity"`
	Behavior     BehaviorConfig     `mapstructure:"behavior"`
	Relationship RelationshipConfig `mapstructure:"relationship"`

	// Extensibility: capture any custom fields user adds
	Custom map[string]interface{} `mapstructure:",remain"`
}

type IdentityConfig struct {
	Essence string `mapstructure:"essence"`
	Role    string `mapstructure:"role"`
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
	Work         UserWork        `mapstructure:"work"`
	DailyRoutine DailyRoutine    `mapstructure:"daily_routine"`
	Preferences  UserPreferences `mapstructure:"preferences"`
	Learned      UserLearned     `mapstructure:"learned"`
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

type UserWork struct {
	CurrentProjects []Project `mapstructure:"current_projects"`
	Goals           Goals     `mapstructure:"goals"`
}

type Goals struct {
	ShortTerm []string `mapstructure:"short_term"`
	LongTerm  []string `mapstructure:"long_term"`
}

type DailyRoutine struct {
	WorkingHours  string `mapstructure:"working_hours"`
	SleepingHours string `mapstructure:"sleeping_hours"`
}

type Project struct {
	Name        string `mapstructure:"name"`
	Description string `mapstructure:"description"`
	Status      string `mapstructure:"status"`
	Priority    string `mapstructure:"priority"`
}
type UserPreferences struct {
	Technical     []string `mapstructure:"technical"`
	Collaboration []string `mapstructure:"collaboration"`
	WhatIValue    []string `mapstructure:"what_i_value"`
}

type UserLearned struct {
	Likes    []string `mapstructure:"likes"`
	Dislikes []string `mapstructure:"dislikes"`
	Patterns []string `mapstructure:"patterns"`
	Context  []string `mapstructure:"context"`
}

// GetVault returns the vault instance attached to this config.
func (c *Config) GetVault() vault.Vault { return c.v }

// setVault attaches a vault instance (used by loaders only).
func (c *Config) setVault(v vault.Vault) { c.v = v }

// Close releases vault resources.
func (c *Config) Close() error {
	if c.v != nil {
		return c.v.Close()
	}
	return nil
}

// Validate checks required fields are present.
func (c *Config) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("agent.id is required")
	}
	if c.LLM.Provider == "" {
		return fmt.Errorf("llm.provider is required")
	}
	if c.LLM.APIKeyVault == "" {
		return fmt.Errorf("llm.api_key_vault is required (vault path to API key)")
	}
	return nil
}
