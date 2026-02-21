package agents

import (
	"context"
	"log/slog"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/config"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/soul"
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
	maxConcurrency int

	// Core capabilities
	llmClient llm.Client
	soul      *soul.Soul
	user      *config.UserConfig

	// Tool definitions given to LLM (schema only, no implementation)
	// Kernel populates this based on which skills are assigned to this agent.
	tools []tools.ToolDef

	// Kernel bus — agent fires events here (tool calls, outbound messages)
	outbox chan<- core.Event

	// Inbox — kernel sends events here (execute requests, tool results)
	inbox chan core.Event

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *slog.Logger
}

// AgentRegistry holds all running agents. Lives in kernel.
type Registry struct {
	agents map[string]*Agent
	mu     sync.RWMutex
}
