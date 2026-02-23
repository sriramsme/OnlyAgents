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
	"database/sql"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
)

// Kernel is the central router. It wires everything together and owns the event bus.
type Kernel struct {
	bus        chan core.Event
	agents     *agents.Registry
	skills     *skills.Registry
	connectors *connectors.Registry
	channels   *channels.Registry
	workflow   *core.Engine

	// defaultAgentID is used when a channel message doesn't specify a target agent
	defaultAgentID string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *slog.Logger
}

// Config is passed to NewKernel to configure it.
type Config struct {
	BusBufferSize       int
	DefaultAgentID      string
	AgentConfigsDir     string
	ConnectorConfigsDir string
	ChannelConfigsDir   string
	SkillConfigsDir     string
	VaultPath           string
}

func NewKernel(cfg Config, ctx context.Context, cancel context.CancelFunc) (*Kernel, error) {
	if cfg.BusBufferSize == 0 {
		cfg.BusBufferSize = 256
	}
	if cfg.AgentConfigsDir == "" {
		cfg.AgentConfigsDir = "configs/agents/"
	}
	if cfg.ConnectorConfigsDir == "" {
		cfg.ConnectorConfigsDir = "configs/connectors/"
	}
	if cfg.ChannelConfigsDir == "" {
		cfg.ChannelConfigsDir = "configs/channels/"
	}
	if cfg.SkillConfigsDir == "" {
		cfg.SkillConfigsDir = "configs/skills/"
	}
	if cfg.VaultPath == "" {
		cfg.VaultPath = "configs/vault.yaml"
	}

	kernelBus := make(chan core.Event, cfg.BusBufferSize)

	v, err := loadVault(cfg.VaultPath)
	if err != nil {
		return nil, fmt.Errorf("load vault: %w", err)
	}

	agentsRegistry, err := loadAgents(ctx, v, cfg.AgentConfigsDir, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("load agents: %w", err)
	}

	connectorsRegistry, err := loadConnectors(ctx, v, cfg.ConnectorConfigsDir, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("load connectors: %w", err)
	}

	channelsRegistry, err := loadChannels(ctx, v, cfg.ChannelConfigsDir, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("load channels: %w", err)
	}

	skillsRegistry, err := loadSkills(ctx, cfg.SkillConfigsDir, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}

	if err := validateAgentSkills(agentsRegistry, skillsRegistry); err != nil {
		return nil, fmt.Errorf("validate agent skills: %w", err)
	}

	// Initialize workflow engine
	db, err := sql.Open("sqlite", "workflows.db")
	if err != nil {
		return nil, err
	}

	workflowEngine, err := core.NewEngine(db, kernelBus)
	if err != nil {
		return nil, err
	}

	return &Kernel{
		bus:            kernelBus,
		agents:         agentsRegistry,
		skills:         skillsRegistry,
		connectors:     connectorsRegistry,
		channels:       channelsRegistry,
		workflow:       workflowEngine,
		defaultAgentID: cfg.DefaultAgentID,
		ctx:            ctx,
		cancel:         cancel,
		logger:         slog.Default().With("component", "kernel"),
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

// --- Lifecycle ---

func (k *Kernel) Start() error {
	k.logger.Info("starting kernel")

	// Start all channels — they'll write MessageReceived events to the bus
	for _, ch := range k.channels.All() {
		if err := ch.Start(); err != nil {
			return fmt.Errorf("failed to start channel %s: %w", ch.PlatformName(), err)
		}
	}

	// Start all connectors
	for _, c := range k.connectors.All() {
		if err := c.Start(); err != nil {
			return fmt.Errorf("failed to start connector %s: %w", c.Name(), err)
		}
	}

	// Start all agents
	for _, a := range k.agents.All() {
		if err := a.Start(); err != nil {
			return fmt.Errorf("failed to start agent %s: %w", a.ID(), err)
		}
	}

	// Start all skills
	if err := k.initializeSkills(); err != nil {
		return fmt.Errorf("failed to initialize skills: %w", err)
	}

	// assign agent tools
	if err := k.assignAgentTools(); err != nil {
		return fmt.Errorf("failed to assign agent tools: %w", err)
	}

	// Start event router
	k.wg.Add(1)
	go k.run()

	k.logger.Info("kernel started")
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

	// Future
	case core.AgentMessage:
		k.handleAgentMessage(evt)

	default:
		k.logger.Warn("unhandled event type", "type", evt.Type)
	}
}
