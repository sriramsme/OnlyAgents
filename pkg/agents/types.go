package agents

import (
	"context"
	"strings"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// public agent interface
type Instance interface {
	ID() string
	Name() string
	Description() string

	Start() error
	Stop() error

	Inbox() chan<- core.Event
	Status() core.AgentStatus

	IsExecutive() bool
	IsGeneral() bool

	SetTools(tools []tools.ToolDef)
	AddTools(tools []tools.ToolDef)
	AttachSkill(skill skills.Skill) error
	AddSkill(skill skills.Skill)
	GetSkillNames() []string
	ListToolNames() []string
	GetSkillBindings() []SkillBinding

	SetHandleFindSkill(fn handleFindSkillFunc)
	SetResolveAgentName(fn AgentNameResolver)
	SetUserContext(userContext string)
	SetAvailableAgents(agents map[string]AgentInfo) // only populated for executive
	RegisterPeer(agentInfo AgentInfo)               // no-op for non-executive
	RebuildSystemPrompt()
}

type AgentInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	IsGeneral    bool     `json:"is_general,omitempty"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

// AgentRegistry holds all running agents. Lives in kernel.
type Registry struct {
	agents    map[string]Instance
	executive *Agent
	general   *Agent
	mu        sync.RWMutex
}

// Personality defines core personality traits
type Personality struct {
	Archetype  string   // e.g., "dedicated_assistant", "creative_researcher"
	Traits     []string // e.g., "loyal", "efficient", "warm", "professional"
	Tone       string   // e.g., "friendly", "formal", "casual"
	Humor      bool     // Can use humor
	Submissive bool     // Defers to user, doesn't argue
}

// CommunicationStyle defines how the agent communicates
type CommunicationStyle struct {
	Formality       string // "casual", "professional", "formal"
	Verbosity       string // "concise", "balanced", "detailed"
	UseEmoji        bool
	AddressUser     string   // e.g., "boss", "friend", "sir", ""
	Acknowledgments []string // e.g., "Got it!", "On it!", "Understood!"
}

// CoreValues defines foundational principles (not just traits)
type CoreValues struct {
	Primary    map[string]string // e.g., "honesty_over_sycophancy": "explanation"
	Boundaries []string          // Hard limits and principles
	Hierarchy  []string          // Ordered priorities when values conflict
}

// Relationship defines connection to user
type Relationship struct {
	ToUser      string   // Nature of relationship
	Trust       string   // How trust is built
	Growth      string   // How relationship evolves
	Boundaries  []string // Relational boundaries
	Partnership string   // Collaborative principles
}

// Purpose captures why the agent exists
type Purpose struct {
	WhyIExist  string   // Fundamental reason for being
	WhatIValue []string // Core priorities
	Philosophy string   // Guiding worldview
}
type toolCallBuilder struct {
	ID   string
	Name string
	Args strings.Builder
}
type (
	handleFindSkillFunc func(ctx context.Context, a Instance, skillName string) (any, error)
	AgentNameResolver   func(agentID string) string
)
