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

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
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
}

func NewKernel(cfg Config, v vault.Vault, ctx context.Context, cancel context.CancelFunc) (*Kernel, error) {
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

	kernelBus := make(chan core.Event, cfg.BusBufferSize)

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

	skillsRegistry, err := loadSkills(ctx, v, cfg.SkillConfigsDir, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
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
		if err := ch.Start(k.ctx); err != nil {
			return fmt.Errorf("failed to start channel %s: %w", ch.PlatformName(), err)
		}
	}

	// Start all connectors
	for _, c := range k.connectors.All() {
		if err := c.Start(k.ctx); err != nil {
			return fmt.Errorf("failed to start connector %s: %w", c.Name(), err)
		}
	}

	// Start all agents
	for _, a := range k.agents.All() {
		if err := a.Start(); err != nil {
			return fmt.Errorf("failed to start agent %s: %w", a.ID(), err)
		}
	}

	// Start event router
	k.wg.Add(1)
	go k.run()

	k.logger.Info("kernel started")
	return nil
}

func (k *Kernel) Stop() error {
	k.logger.Info("stopping kernel")
	k.cancel()
	k.wg.Wait()
	return nil
}

// --- Event router ---

func (k *Kernel) run() {
	defer k.wg.Done()
	for {
		select {
		case evt := <-k.bus:
			k.route(evt)
		case <-k.ctx.Done():
			return
		}
	}
}

func (k *Kernel) route(evt core.Event) {
	k.logger.Debug("routing event", "type", evt.Type, "correlation_id", evt.CorrelationID)

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
		k.logger.Error("invalid MessageReceived payload")
		return
	}

	// Resolve target agent (could be from channel config, metadata, or default)
	agentID := evt.AgentID

	if agentID == "" {
		agentID = k.defaultAgentID
	}

	if id, ok := payload.Metadata["target_agent"]; ok && id != "" {
		agentID = id
	}

	agent, err := k.agents.Get(agentID)
	if err != nil {
		k.logger.Error("target agent not found", "agent_id", agentID)
		return
	}

	correlationID := evt.CorrelationID
	if correlationID == "" {
		correlationID = uuid.NewString()
	}

	agent.Inbox() <- core.Event{
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
}

// handleToolCallRequest: agent wants to execute a tool, kernel dispatches to the right skill
func (k *Kernel) handleToolCallRequest(evt core.Event) {
	payload, ok := evt.Payload.(core.ToolCallRequestPayload)
	if !ok {
		k.logger.Error("invalid ToolCallRequest payload")
		return
	}

	skill, ok := k.skills.Get(payload.SkillName)
	if !ok {
		k.sendToolError(evt, fmt.Sprintf("skill not found: %s", payload.SkillName))
		return
	}

	// Execute skill in a goroutine so we don't block the router
	go func() {
		result, err := skill.Execute(k.ctx, payload.ToolName, payload.Params)

		resultEvt := core.Event{
			Type:          core.ToolCallResult,
			CorrelationID: evt.CorrelationID,
			AgentID:       evt.AgentID,
		}

		if err != nil {
			resultEvt.Payload = core.ToolCallResultPayload{
				ToolCallID: payload.ToolCallID,
				ToolName:   payload.ToolName,
				Error:      err.Error(),
			}
		} else {
			resultEvt.Payload = core.ToolCallResultPayload{
				ToolCallID: payload.ToolCallID,
				ToolName:   payload.ToolName,
				Result:     result,
			}
		}

		// Reply directly to the agent's waiting goroutine via the ReplyTo channel
		if evt.ReplyTo != nil {
			evt.ReplyTo <- resultEvt
		}
	}()
}

// handleOutboundMessage: agent has a response, send it via the appropriate channel
func (k *Kernel) handleOutboundMessage(evt core.Event) {
	payload, ok := evt.Payload.(core.OutboundMessagePayload)
	if !ok {
		k.logger.Error("invalid OutboundMessage payload")
		return
	}

	ch, err := k.channels.Get(payload.ChannelName)
	if err != nil {
		k.logger.Error("channel not found", "channel", payload.ChannelName)
		return
	}

	if err := ch.Send(k.ctx, channels.OutgoingMessage{
		ChatID:    payload.ChatID,
		Content:   payload.Content,
		ReplyToID: payload.ReplyToID,
		ParseMode: payload.ParseMode,
	}); err != nil {
		k.logger.Error("failed to send outbound message",
			"channel", payload.ChannelName,
			"error", err)
	}
}

// handleAgentRequest: a skill needs a sub-agent to perform a task
func (k *Kernel) handleAgentRequest(evt core.Event) {
	payload, ok := evt.Payload.(core.AgentRequestPayload)
	if !ok {
		k.logger.Error("invalid AgentRequest payload")
		return
	}

	// For now, route to default executive agent.
	// Later: could spawn a dedicated sub-agent based on task type.
	agent, err := k.agents.Get(k.defaultAgentID)
	if err != nil {
		k.logger.Error("no agent available for sub-agent request")
		return
	}

	agent.Inbox() <- core.Event{
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
}

func (k *Kernel) sendToolError(evt core.Event, msg string) {
	if evt.ReplyTo == nil {
		return
	}
	payload := evt.Payload.(core.ToolCallRequestPayload)
	evt.ReplyTo <- core.Event{
		Type:          core.ToolCallResult,
		CorrelationID: evt.CorrelationID,
		Payload: core.ToolCallResultPayload{
			ToolCallID: payload.ToolCallID,
			ToolName:   payload.ToolName,
			Error:      msg,
		},
	}
}
