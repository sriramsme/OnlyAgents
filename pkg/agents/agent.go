// Agent execution model:
//
//  Sync path  (HTTP handler → Execute):
//    Execute() builds messages, calls LLM, fires ToolCallRequest events to kernel,
//    blocks on reply channel until kernel returns ToolCallResult, then resumes LLM loop.
//
//  Async path (A2A / kernel → agent):
//    Kernel sends AgentExecute event to agent.inbox.
//    processEvents() picks it up and calls execute() internally.
//    Outbound response goes back as OutboundMessage event → kernel → channel.

package agents

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

type Agent struct {
	id               string
	name             string
	description      string
	isExecutive      bool
	isGeneral        bool
	maxConcurrency   int
	streamingEnabled bool

	soul            *Soul
	userContext     string
	availableAgents map[string]AgentInfo // only populated for executive
	systemPrompt    string               // always the assembled result, never set externally

	// Core capabilities
	llmClient      llm.Client
	skillsBindings []config.SkillBinding
	skills         map[string]skills.Skill // owns lifecycle

	// Tool definitions given to LLM (schema only, no implementation)
	// Kernel populates this based on which skills are assigned to this agent.
	tools        []tools.ToolDef
	toolSkillMap map[string]string
	activeGroups map[string]map[string][]tools.ToolGroup // session → skill → groups

	// Kernel bus — agent fires events here (tool calls, outbound messages)
	outbox chan<- core.Event

	// Inbox — kernel sends events here (execute requests, tool results)
	inbox chan core.Event

	cm *memory.ConversationManager // shared across all agents, injected by kernel
	mm *memory.MemoryManager       // shared across all agents, injected by kernel

	handleFindSkill  handleFindSkillFunc // injected by kernel only for general agents
	resolveAgentName AgentNameResolver

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *slog.Logger

	// UI observability — nil in headless mode, zero overhead when unset.
	uiBus       chan<- core.UIEvent
	activeSince time.Time // when the current task started; used for idle duration

	executeMu sync.Map // map[sessionID]*sync.Mutex — serializes turns per session

	// Runtime state — owned by the agent, read by KernelReader.Agents()
	stateMu     sync.RWMutex
	state       core.AgentState
	currentTask string
	lastActive  time.Time
}

// NewAgent creates an agent. Kernel calls this and injects the shared bus + tool definitions.
func NewAgent(
	ctx context.Context, // ← Parent context (kernel's context)
	cfg config.Agent,
	llmClient llm.Client,
	outbox chan<- core.Event,
	uiBus core.UIBus,
	cm *memory.ConversationManager,
	mm *memory.MemoryManager,
) (*Agent, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("llm client is required")
	}

	agentSoul := NewSoul(cfg.Soul)

	// Create agent context from parent - ties agent lifecycle to kernel
	agentCtx, cancel := context.WithCancel(ctx) // #nosec G118 -- cancel is called in Stop()

	return &Agent{
		id:               cfg.ID,
		name:             cfg.Name,
		description:      cfg.Description,
		isExecutive:      cfg.IsExecutive,
		isGeneral:        cfg.IsGeneral,
		maxConcurrency:   cfg.MaxConcurrency,
		skillsBindings:   cfg.Skills,
		streamingEnabled: cfg.StreamingEnabled,
		llmClient:        llmClient,
		soul:             agentSoul,
		outbox:           outbox,
		uiBus:            uiBus,
		tools:            []tools.ToolDef{},
		skills:           make(map[string]skills.Skill),
		toolSkillMap:     make(map[string]string),
		availableAgents:  make(map[string]AgentInfo), // only populated for executive
		cm:               cm,
		mm:               mm,
		inbox:            make(chan core.Event, cfg.BufferSize),
		ctx:              agentCtx,
		cancel:           cancel,
		logger:           slog.With("agent_id", cfg.ID),
		state:            "idle",
	}, nil
}
