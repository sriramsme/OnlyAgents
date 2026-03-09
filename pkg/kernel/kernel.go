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
	"runtime/debug"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sriramsme/OnlyAgents/internal/bootstrap"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/channels/oaChannel"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/connectors/native"
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
	capabilities            *core.CapabilityRegistry
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

	buildSystemPrompts(components.user, components.agents, components.capabilityMap)

	fmt.Println("kernel created")
	fmt.Println("skills: ", components.skills.ListAll())
	fmt.Println("agents: ", components.agents.ListAll())
	fmt.Println("capabilities: ", components.capabilities.ListAll())

	return &Kernel{
		bus:                     kernelBus,
		agents:                  components.agents,
		skills:                  components.skills,
		connectors:              components.connectors,
		channels:                components.channels,
		user:                    components.user,
		workflow:                components.workflow,
		capabilities:            components.capabilities,
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

func (k *Kernel) RegisterSkill(s skills.Skill) error {
	return k.skills.Register(s)
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

func (k *Kernel) OAChannel() *oaChannel.OAChannel {
	ch, err := k.channels.Get("onlyagents")
	if err != nil {
		k.logger.Error("failed to get channel", "name", "oaChannel", "err", err)
		return nil
	}

	oaCh, ok := ch.(*oaChannel.OAChannel)
	if !ok {
		k.logger.Error("channel type mismatch", "expected", "*oaChannel.OAChannel")
		return nil
	}

	return oaCh
}

func (k *Kernel) initNativeConnectors() {
	k.RegisterConnector(native.NewCalendarConnector(k.store))
	k.RegisterConnector(native.NewNotesConnector(k.store))
	k.RegisterConnector(native.NewRemindersConnector(k.store))
	k.RegisterConnector(native.NewTasksConnector(k.store))
}

// --- Lifecycle ---

func (k *Kernel) Start() error {
	k.logger.Info("starting kernel")

	k.logger.Info("initializing native connectors")
	k.initNativeConnectors()

	k.logger.Info("initializing skills")
	if err := k.initializeSkills(); err != nil {
		return fmt.Errorf("failed to initialize skills: %w", err)
	}

	// Start all channels — they'll write MessageReceived events to the bus
	k.logger.Info("starting channels")
	for _, ch := range k.channels.All() {
		if err := ch.Start(); err != nil {
			return fmt.Errorf("failed to start channel %s: %w", ch.PlatformName(), err)
		}
	}

	k.logger.Info("wiring onlyagents Channel")
	k.wireOAChannel()

	// Start all connectors
	k.logger.Info("starting connectors")
	for _, c := range k.connectors.All() {
		if err := c.Start(); err != nil {
			return fmt.Errorf("failed to start connector %s: %w", c.Name(), err)
		}
	}

	// Start all agents
	k.logger.Info("starting agents")
	for _, a := range k.agents.All() {
		if err := a.Start(); err != nil {
			return fmt.Errorf("failed to start agent %s: %w", a.ID(), err)
		}
	}

	// Start workflow engine
	k.logger.Info("starting workflow engine")
	if err := k.workflow.Start(); err != nil {
		return fmt.Errorf("failed to start workflow engine: %w", err)
	}
	k.workflow.SetAgentFinder(k.findBestAgentToolDep)

	// assign agent tools
	k.logger.Info("assigning agent tools")
	if err := k.assignAgentTools(); err != nil {
		return fmt.Errorf("failed to assign agent tools: %w", err)
	}

	// Start event router
	k.wg.Add(1)
	go k.run()

	if k.uiBus != nil {
		k.wg.Add(1)
		go k.runUI()
	}

	k.logger.Info("kernel started")
	fmt.Println("kernel started")
	return nil
}

func (k *Kernel) Stop() error {
	k.logger.Info("stopping kernel - beginning graceful shutdown")

	// Step 1: Stop accepting new events - stop all channels first
	k.logger.Info("stopping channels to prevent new messages")
	for _, ch := range k.channels.All() {
		if err := ch.Stop(); err != nil {
			k.logger.Error("failed to stop channel",
				"channel", ch.PlatformName(),
				"error", err)
		}
	}

	// Step 2: Allow time for in-flight events to be processed
	k.logger.Info("draining event bus")
	drainTimer := time.NewTimer(2 * time.Second)
	select {
	case <-drainTimer.C:
		k.logger.Info("drain period complete", "pending_events", len(k.bus))
	case <-k.ctx.Done():
		// Already cancelled from outside
	}

	// Step 3: Cancel context to signal shutdown to router and components
	k.logger.Info("cancelling context")
	k.cancel()

	// Step 4: Wait for event router and tool call goroutines
	k.logger.Info("waiting for event router and goroutines to complete")
	done := make(chan struct{})
	go func() {
		k.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		k.logger.Info("all goroutines stopped gracefully")
	case <-time.After(10 * time.Second):
		k.logger.Error("kernel shutdown timeout - some goroutines may be leaked",
			"warning", "check for blocked channels or infinite loops")
	}

	// Step 5: Stop agents
	k.logger.Info("stopping agents")
	for _, a := range k.agents.All() {
		if err := a.Stop(); err != nil {
			k.logger.Error("failed to stop agent",
				"agent_id", a.ID(),
				"error", err)
		}
	}

	// Step 6: Stop connectors
	k.logger.Info("stopping connectors")
	for _, c := range k.connectors.All() {
		if err := c.Stop(); err != nil {
			k.logger.Error("failed to stop connector",
				"connector", c.Name(),
				"error", err)
		}
	}

	defer func() {
		if err := k.store.Close(); err != nil {
			k.logger.Error("storage close failed", "err", err)
		}
	}()

	k.logger.Info("kernel stopped")
	return nil
}

// --- Event router ---

func (k *Kernel) run() {
	defer k.wg.Done()

	for {
		select {
		case evt := <-k.bus:
			// Wrap route() in panic recovery
			func() {
				defer func() {
					if r := recover(); r != nil {
						k.logger.Error("PANIC in event router",
							"panic", r,
							"event_type", evt.Type,
							"correlation_id", evt.CorrelationID,
							"agent_id", evt.AgentID,
							"stack", string(debug.Stack()))

						// Try to send error response if this was a request
						if evt.ReplyTo != nil {
							errorEvt := core.Event{
								Type:          evt.Type,
								CorrelationID: evt.CorrelationID,
								Payload: core.ErrorPayload{
									Error: "internal error processing event",
								},
							}

							select {
							case evt.ReplyTo <- errorEvt:
								k.logger.Info("sent error response to requester")
							default:
								k.logger.Warn("could not send error response - channel full or closed")
							}
						}
					}
				}()

				k.route(evt)
			}()

		case <-k.ctx.Done():
			k.logger.Info("event router shutting down")
			return
		}
	}
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

	// Tool execution
	case core.ToolCallRequest:
		k.handleToolCallRequest(evt)

	case core.ToolCallResult:
		// Results typically go directly via ReplyTo channel
		// If it arrives here, it's an error
		k.logger.Warn("ToolCallResult should not arrive at bus",
			"correlation_id", evt.CorrelationID)

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

//reads UIBus, fans out to all SSE subscribers

func (k *Kernel) runUI() {
	defer k.wg.Done()
	for {
		select {
		case evt := <-k.uiBus:
			k.broadcastUI(evt)
		case <-k.ctx.Done():
			k.logger.Info("ui event router shutting down")
			return
		}
	}
}

// non-blocking fan-out to all SSE subscriber channels
func (k *Kernel) broadcastUI(evt core.UIEvent) {
	k.uiSubsMu.RLock()
	defer k.uiSubsMu.RUnlock()
	for _, ch := range k.uiSubs {
		select {
		case ch <- evt:
		default: // slow client — drop rather than block the fan-out loop
		}
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

// implements KernelReader
func (k *Kernel) Subscribe(id string) (<-chan core.UIEvent, func()) {
	ch := make(chan core.UIEvent, 64)
	k.uiSubsMu.Lock()
	k.uiSubs[id] = ch
	k.uiSubsMu.Unlock()

	return ch, func() {
		k.uiSubsMu.Lock()
		delete(k.uiSubs, id)
		k.uiSubsMu.Unlock()
	}
}

// implements KernelReader
func (k *Kernel) AgentsStatus() []core.AgentStatus {
	ids := k.agents.ListAll()
	out := make([]core.AgentStatus, 0, len(ids))
	for _, id := range ids {
		a, err := k.agents.Get(id)
		if err != nil {
			continue
		}
		out = append(out, a.Status())
	}
	return out
}

// implements KernelReader
func (k *Kernel) IsHealthy() bool {
	return k.ctx.Err() == nil
}
