package agents

import (
	"context"
	"log/slog"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// Agent is a pure execution unit.
// It knows about: LLM, Soul, tools (as definitions only), and two channels.
// It knows nothing about skills, connectors, or other agents directly.
// Kernel injects tool definitions at construction; all tool calls go back through kernel.
type Agent struct {
	id             string
	name           string
	isExecutive    bool
	isGeneral      bool
	maxConcurrency int

	// Core capabilities
	llmClient llm.Client
	soul      *Soul
	skills    []tools.SkillName

	// Tool definitions given to LLM (schema only, no implementation)
	// Kernel populates this based on which skills are assigned to this agent.
	tools        []tools.ToolDef
	toolSkillMap map[string]tools.SkillName

	// Kernel bus — agent fires events here (tool calls, outbound messages)
	outbox chan<- core.Event

	// Inbox — kernel sends events here (execute requests, tool results)
	inbox chan core.Event

	cm *memory.ConversationManager // shared across all agents, injected by kernel
	mm *memory.MemoryManager       // shared across all agents, injected by kernel

	systemPrompt  string
	findBestAgent tools.FindBestAgentFunc // injected by kernel only for executive agents
	findSkill     findSkillFunc           // injected by kernel only for general agents
	useSkillTool  useSkillToolFunc        // injected by kernel only for general agents

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *slog.Logger
}

// AgentRegistry holds all running agents. Lives in kernel.
type Registry struct {
	agents    map[string]*Agent
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

type findSkillFunc func(ctx context.Context, capability core.Capability) (interface{}, error)
type useSkillToolFunc func(ctx context.Context, skillName tools.SkillName, toolName string, params map[string]interface{}) (interface{}, error)
