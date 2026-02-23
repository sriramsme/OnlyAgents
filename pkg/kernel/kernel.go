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

	"github.com/google/uuid"
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

	return &Kernel{
		bus:            kernelBus,
		agents:         agentsRegistry,
		skills:         skillsRegistry,
		connectors:     connectorsRegistry,
		channels:       channelsRegistry,
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
	case core.MessageReceived:
		k.handleMessageReceived(evt)

	case core.ToolCallRequest:
		k.handleToolCallRequest(evt)

	case core.OutboundMessage:
		k.handleOutboundMessage(evt)

	case core.AgentRequest:
		k.handleAgentRequest(evt)

	default:
		k.logger.Warn("unhandled event type", "type", evt.Type)
	}
}

// handleMessageReceived: channel got a user message, route to appropriate agent
func (k *Kernel) handleMessageReceived(evt core.Event) {
	payload, ok := evt.Payload.(core.MessageReceivedPayload)
	if !ok {
		k.logger.Error("invalid MessageReceived payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload))
		return
	}

	// Resolve target agent (could be from channel config, metadata, or default)
	agentID := evt.AgentID
	var err error

	if id, ok := payload.Metadata["target_agent"]; ok && id != "" {
		agentID = id
	}

	if agentID == "" {
		agentID, _ = k.findSpecializedAgent([]core.Capability{})
	}

	agent, err := k.agents.Get(agentID)
	if err != nil {
		k.logger.Error("target agent not found",
			"agent_id", agentID,
			"correlation_id", evt.CorrelationID)
		return
	}

	correlationID := evt.CorrelationID
	if correlationID == "" {
		correlationID = uuid.NewString()
	}

	agentEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: correlationID,
		AgentID:       agentID,
		Payload: core.AgentExecutePayload{
			UserMessage: payload.Content,
			ChatID:      payload.ChatID,
			Metadata: map[string]string{
				"channel":  payload.ChannelName,
				"user_id":  payload.UserID,
				"username": payload.Username,
			},
		},
	}

	// NON-BLOCKING SEND: Prevents kernel deadlock if agent inbox is full
	select {
	case agent.Inbox() <- agentEvent:
		// Success
		k.logger.Debug("dispatched to agent", "agent_id", agentID)

	case <-time.After(5 * time.Second):
		k.logger.Error("agent inbox full - message dropped",
			"agent_id", agentID,
			"correlation_id", correlationID,
			"action", "consider increasing agent buffer size or investigating slow processing")

		// TODO: Send error response back to channel
		// Could implement a retry queue here

	case <-k.ctx.Done():
		k.logger.Info("shutdown in progress - message not delivered",
			"correlation_id", correlationID)
		return
	}
}

// handleToolCallRequest: agent wants to execute a tool, kernel dispatches to the right skill
func (k *Kernel) handleToolCallRequest(evt core.Event) {
	payload, ok := evt.Payload.(core.ToolCallRequestPayload)
	if !ok {
		k.logger.Error("invalid ToolCallRequest payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload))
		return
	}

	skill, ok := k.skills.Get(payload.SkillName)
	if !ok {
		k.sendToolError(evt, fmt.Sprintf("skill not found: %s", payload.SkillName))
		return
	}

	// TRACKED GOROUTINE: Ensures graceful shutdown waits for tool calls
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()

		// Create timeout context for skill execution
		ctx, cancel := context.WithTimeout(k.ctx, 30*time.Second)
		defer cancel()

		k.logger.Debug("executing skill",
			"skill", payload.SkillName,
			"tool", payload.ToolName,
			"correlation_id", evt.CorrelationID)

		result, err := skill.Execute(ctx, payload.ToolName, payload.Params)

		resultEvt := core.Event{
			Type:          core.ToolCallResult,
			CorrelationID: evt.CorrelationID,
			AgentID:       evt.AgentID,
		}

		if err != nil {
			k.logger.Error("skill execution failed",
				"skill", payload.SkillName,
				"tool", payload.ToolName,
				"error", err,
				"correlation_id", evt.CorrelationID)

			resultEvt.Payload = core.ToolCallResultPayload{
				ToolCallID: payload.ToolCallID,
				ToolName:   payload.ToolName,
				Error:      err.Error(),
			}
		} else {
			k.logger.Debug("skill execution succeeded",
				"skill", payload.SkillName,
				"tool", payload.ToolName,
				"correlation_id", evt.CorrelationID)

			resultEvt.Payload = core.ToolCallResultPayload{
				ToolCallID: payload.ToolCallID,
				ToolName:   payload.ToolName,
				Result:     result,
			}
		}

		// SAFE SEND: Reply directly to the agent's waiting goroutine
		if evt.ReplyTo != nil {
			select {
			case evt.ReplyTo <- resultEvt:
				// Success
			case <-time.After(5 * time.Second):
				k.logger.Error("failed to send tool result - reply channel blocked",
					"tool", payload.ToolName,
					"correlation_id", evt.CorrelationID,
					"warning", "agent may have timed out or shut down")
			case <-k.ctx.Done():
				k.logger.Info("shutdown in progress - tool result not delivered",
					"correlation_id", evt.CorrelationID)
			}
		} else {
			k.logger.Warn("tool call request missing ReplyTo channel",
				"tool", payload.ToolName,
				"correlation_id", evt.CorrelationID)
		}
	}()
}

// handleOutboundMessage: agent has a response, send it via the appropriate channel
func (k *Kernel) handleOutboundMessage(evt core.Event) {
	payload, ok := evt.Payload.(core.OutboundMessagePayload)
	if !ok {
		k.logger.Error("invalid OutboundMessage payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload))
		return
	}

	ch, err := k.channels.Get(payload.ChannelName)
	if err != nil {
		k.logger.Error("channel not found",
			"channel", payload.ChannelName,
			"correlation_id", evt.CorrelationID)
		return
	}

	// Create timeout context for channel send
	ctx, cancel := context.WithTimeout(k.ctx, 10*time.Second)
	defer cancel()

	if err := ch.Send(ctx, channels.OutgoingMessage{
		ChatID:    payload.ChatID,
		Content:   payload.Content,
		ReplyToID: payload.ReplyToID,
		ParseMode: payload.ParseMode,
	}); err != nil {
		k.logger.Error("failed to send outbound message",
			"channel", payload.ChannelName,
			"correlation_id", evt.CorrelationID,
			"error", err)
	} else {
		k.logger.Debug("outbound message sent",
			"channel", payload.ChannelName,
			"correlation_id", evt.CorrelationID)
	}
}

// handleAgentRequest: a skill needs a sub-agent to perform a task
func (k *Kernel) handleAgentRequest(evt core.Event) {
	payload, ok := evt.Payload.(core.AgentRequestPayload)
	if !ok {
		k.logger.Error("invalid AgentRequest payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload))
		return
	}

	// For now, route to default executive agent.
	// Later: could spawn a dedicated sub-agent based on task type.
	agent, err := k.agents.Get(k.defaultAgentID)
	if err != nil {
		k.logger.Error("no agent available for sub-agent request",
			"correlation_id", evt.CorrelationID)
		return
	}

	agentEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: evt.CorrelationID,
		AgentID:       agent.ID(),
		Payload: core.AgentExecutePayload{
			UserMessage: payload.Task,
			Metadata: map[string]string{
				"requesting_skill": payload.RequestingSkill,
				"context":          fmt.Sprintf("%v", payload.Context),
			},
		},
	}

	// NON-BLOCKING SEND
	select {
	case agent.Inbox() <- agentEvent:
		k.logger.Debug("dispatched sub-agent request",
			"agent_id", agent.ID(),
			"requesting_skill", payload.RequestingSkill)
	case <-time.After(5 * time.Second):
		k.logger.Error("failed to dispatch sub-agent request - inbox full",
			"agent_id", agent.ID(),
			"correlation_id", evt.CorrelationID)
	case <-k.ctx.Done():
		k.logger.Info("shutdown in progress - sub-agent request not delivered")
	}
}

func (k *Kernel) sendToolError(evt core.Event, msg string) {
	if evt.ReplyTo == nil {
		k.logger.Warn("cannot send tool error - no reply channel",
			"error", msg,
			"correlation_id", evt.CorrelationID)
		return
	}

	payload, ok := evt.Payload.(core.ToolCallRequestPayload)
	if !ok {
		k.logger.Error("cannot send tool error - invalid payload type")
		return
	}

	errorEvt := core.Event{
		Type:          core.ToolCallResult,
		CorrelationID: evt.CorrelationID,
		Payload: core.ToolCallResultPayload{
			ToolCallID: payload.ToolCallID,
			ToolName:   payload.ToolName,
			Error:      msg,
		},
	}

	select {
	case evt.ReplyTo <- errorEvt:
		// Success
	case <-time.After(time.Second):
		k.logger.Error("failed to send tool error response - timeout",
			"error", msg,
			"correlation_id", evt.CorrelationID)
	case <-k.ctx.Done():
		// Shutdown in progress
	}
}
