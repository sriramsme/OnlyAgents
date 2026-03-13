// Kernel is the composition root and central event router.
//
// It owns all registries and is the ONLY package that imports agents, skills,
// connectors, and channels together. Nothing else does.
//
// Event flow:
//   Channel → bus (MessageReceived)
//   Kernel  → agent.inbox (AgentExecute)
//   Agent   → bus (ToolCallRequest)
//   Kernel  → skill.Execute() → replies via ReplyTo channel
//   Agent   → bus (OutboundMessage)
//   Kernel  → channel.Send()

package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/sriramsme/OnlyAgents/internal/bootstrap"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/channels/oaChannel"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/skills/cli"
	"github.com/sriramsme/OnlyAgents/pkg/skills/marketplace"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

// Kernel is the central router. It wires everything together and owns the event bus.
type Kernel struct {
	bus        chan core.Event
	agents     *agents.Registry
	skills     *skills.Registry
	connectors *connectors.Registry
	channels   *channels.Registry
	workflow   *workflow.Engine
	user       *config.UserConfig

	skillMarketplaceManager *marketplace.Manager
	cliExecutor             *cli.CLIExecutor
	cm                      *memory.ConversationManager
	mm                      *memory.MemoryManager
	store                   storage.Storage

	// defaultAgentID is used when a channel message doesn't specify a target agent
	defaultAgentID string

	// helperClient is used for skill installation
	helperClient llm.Client

	cfg *config.KernelConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *slog.Logger

	// UI fan-out — nil when running headless (no GUI/server).
	// Agents write UIEvents here; runUI() fans them out to SSE subscribers.
	uiBus    core.UIBus
	uiSubsMu sync.RWMutex
	uiSubs   map[string]chan core.UIEvent
}

func NewKernel(ctx context.Context, cancel context.CancelFunc, uiBus core.UIBus) (*Kernel, error) {
	paths, err := bootstrap.Init()
	if err != nil {
		return nil, fmt.Errorf("init paths: %w", err)
	}

	cfg, err := config.LoadKernelConfig()
	if err != nil {
		return nil, fmt.Errorf("load kernel config: %w", err)
	}

	kernelBus := make(chan core.Event, cfg.BusBufferSize)

	components, err := loadComponents(ctx, paths, cfg, kernelBus, uiBus)
	if err != nil {
		return nil, err
	}

	fmt.Println("kernel created")
	fmt.Println("skills: ", components.skills.ListAll())
	fmt.Println("agents: ", components.agents.ListAll())

	return &Kernel{
		bus:                     kernelBus,
		agents:                  components.agents,
		skills:                  components.skills,
		connectors:              components.connectors,
		channels:                components.channels,
		user:                    components.user,
		workflow:                components.workflow,
		skillMarketplaceManager: components.skillMarketplaceManager,
		cliExecutor:             components.cliExecutor,
		cm:                      components.cm,
		mm:                      components.mm,
		store:                   components.store,
		defaultAgentID:          cfg.DefaultAgentID,
		// helperClient:            components.helperClient,
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		logger: slog.Default().With("component", "kernel"),
		uiBus:  uiBus,
		uiSubs: make(map[string]chan core.UIEvent),
	}, nil
}

// --- Registration (called during app bootstrap) ---

func (k *Kernel) RegisterAgent(a *agents.Agent) {
	k.agents.Register(a)
}

func (k *Kernel) RegisterSkill(s config.SkillConfig) {
	k.skills.Register(s)
}

func (k *Kernel) RegisterConnector(c connectors.Connector) {
	k.connectors.Register(c)
}

func (k *Kernel) RegisterChannel(ch channels.Channel) {
	k.channels.Register(ch)
}

// Bus returns the write-end of the event bus.
// Channels and agents write here; kernel reads and routes.
func (k *Kernel) Bus() chan<- core.Event {
	return k.bus
}

// nolint:gocyclo
func (k *Kernel) route(evt core.Event) {
	k.logger.Debug("routing event",
		"type", evt.Type,
		"correlation_id", evt.CorrelationID,
		"agent_id", evt.AgentID)

	switch evt.Type {
	// User-facing
	case core.MessageReceived:
		k.handleMessageReceived(evt)

	case core.OutboundMessage:
		k.handleOutboundMessage(evt)

	// Agent execution
	case core.AgentExecute:
		// This is typically handled directly by agents' inboxes
		// If it arrives here, route it
		k.handleAgentExecute(evt)

	// Delegation
	case core.AgentDelegate:
		k.handleAgentDelegate(evt)

	case core.DelegationResult:
		// Results typically go directly via ReplyTo channel
		k.logger.Debug("delegation result received",
			"correlation_id", evt.CorrelationID)

	// Workflow
	case core.WorkflowSubmitted:
		k.handleWorkflowSubmitted(evt)

	case core.WorkflowCompleted:
		k.handleWorkflowCompleted(evt)

	case core.TaskAssigned:
		k.handleTaskAssigned(evt)

	case core.TaskCompleted:
		k.handleTaskCompleted(evt)

	case core.SessionGet:
		k.handleSessionGet(evt)

	case core.SessionNew:
		k.handleSessionNew(evt)

	case core.SessionEnd:
		k.handleSessionEnd(evt)

	// Future
	case core.AgentMessage:
		k.handleAgentMessage(evt)

	case core.OutboundToken:
		k.handleOutboundToken(evt)

	default:
		k.logger.Warn("unhandled event type", "type", evt.Type)
	}
}

func (k *Kernel) wireOAChannel() {
	ch, err := k.channels.Get("onlyagents")
	if err != nil {
		return
	}
	oaCh, ok := ch.(*oaChannel.OAChannel)
	if !ok {
		return
	}
	// Inject Subscribe so each WS connection gets its own UIBus subscription.
	oaCh.SetSubscribe(k.Subscribe)
	oaCh.SetAgentsStatus(k.AgentsStatus)
}
