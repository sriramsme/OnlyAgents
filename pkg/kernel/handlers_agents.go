package kernel

import (
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// handleAgentExecute: Direct agent execution (rare - usually goes via inbox)
func (k *Kernel) handleAgentExecute(evt core.Event) {
	var payload core.AgentExecutePayload
	if err := core.UnmarshalEventPayload(evt.Payload, &payload); err != nil {
		k.logger.Error("invalid AgentExecute payload", "err", err)
		return
	}
	agent, err := k.agents.Get(evt.AgentID)
	if err != nil {
		k.logger.Error("target agent not found",
			"agent_id", evt.AgentID,
			"correlation_id", evt.CorrelationID)
		return
	}

	// Fallback to active channel if not specified
	// In most cases, ChannelMetadata will be set by the caller
	if payload.Channel == nil {
		k.logger.Error("no channel metadata available for outbound message",
			"correlation_id", evt.CorrelationID)

		channelMetadata, err := k.GetActiveChannelMetadata()
		if err != nil {
			k.logger.Error("failed to get active channel metadata",
				"error", err)
			return
		}
		payload.Channel = channelMetadata
	}

	evt.Payload = payload
	// Forward to agent's inbox
	select {
	case agent.Inbox() <- evt:
		k.logger.Debug("forwarded to agent", "agent_id", evt.AgentID)

	case <-time.After(5 * time.Second):
		k.logger.Error("agent inbox full",
			"agent_id", evt.AgentID,
			"correlation_id", evt.CorrelationID)

	case <-k.ctx.Done():
		return
	}
}

// handleAgentDelegate: Executive wants to delegate a task
func (k *Kernel) handleAgentDelegate(evt core.Event) {
	payload, ok := evt.Payload.(core.AgentDelegatePayload)
	if !ok {
		k.logger.Error("invalid AgentDelegate payload")
		return
	}
	if k.uiBus != nil {
		k.uiBus <- core.UIEvent{
			Type:      core.UIEventDelegation,
			Timestamp: time.Now(),
			AgentID:   evt.AgentID,
			Payload: core.DelegationPayload{
				FromAgent: evt.AgentID,
				ToAgent:   payload.AgentID,
				Task:      payload.Task,
			},
		}
	}
	delegationPhase := fmt.Sprintf("delegation_%s", payload.AgentID)
	logger.Timing.StartPhase(evt.CorrelationID, delegationPhase)

	// Executive specifies agent_id directly (preferred)
	if payload.AgentID == "" {
		logger.Timing.EndPhase(evt.CorrelationID, delegationPhase)
		k.logger.Error("no agent found for delegation",
			"agent_id", payload.AgentID)
		k.sendDelegationError(evt, fmt.Sprintf("No agent found for ID: %s", payload.AgentID))
		return
	}
	targetAgent, err := k.agents.Get(payload.AgentID)
	if err != nil {
		logger.Timing.EndPhase(evt.CorrelationID, delegationPhase)
		k.logger.Error("agent not found for delegation",
			"agent_id", payload.AgentID)
		k.sendDelegationError(evt, fmt.Sprintf("Agent not found: %s", payload.AgentID))
		return
	}
	k.logger.Info("delegating task",
		"from_agent", evt.AgentID,
		"to_agent", targetAgent.ID(),
		"correlation_id", evt.CorrelationID,
		"attachments", len(payload.Attachments))

	// Create execution event for target agent
	delegateEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: evt.CorrelationID,
		AgentID:       targetAgent.ID(),
		Payload: core.AgentExecutePayload{
			Message:     payload.Task,
			MessageType: core.MessageTypeDelegation,
			Channel:     payload.Channel,
			Attachments: payload.Attachments,
			Delegation: &core.DelegationMetadata{
				DelegationID:       payload.DelegationID,
				SendDirectlyToUser: payload.SendDirectlyToUser,
				FromAgentID:        evt.AgentID,
				DelegatedAt:        time.Now(),
			},
		},
		ReplyTo: evt.ReplyTo, // Result goes back to delegating agent
	}

	// Send to target agent
	select {
	case targetAgent.Inbox() <- delegateEvent:
		k.logger.Debug("delegation sent",
			"to_agent", targetAgent.ID(),
			"correlation_id", evt.CorrelationID)
		// Note: We don't end the phase here - it ends when delegation completes

	case <-time.After(5 * time.Second):
		logger.Timing.EndPhase(evt.CorrelationID, delegationPhase)
		k.logger.Error("failed to delegate - agent inbox full",
			"agent_id", targetAgent.ID())
		k.sendDelegationError(evt, "Target agent busy")

	case <-k.ctx.Done():
		logger.Timing.EndPhase(evt.CorrelationID, delegationPhase)
		k.logger.Info("shutdown in progress - delegation not sent")
	}
}

// handleAgentMessage: Direct agent-to-agent communication (future)
func (k *Kernel) handleAgentMessage(evt core.Event) {
	payload, ok := evt.Payload.(core.AgentMessagePayload)
	if !ok {
		k.logger.Error("invalid AgentMessage payload")
		return
	}

	targetAgent, err := k.agents.Get(payload.ToAgent)
	if err != nil {
		k.logger.Error("target agent not found",
			"to_agent", payload.ToAgent,
			"from_agent", payload.FromAgent)
		return
	}

	k.logger.Debug("routing agent message",
		"from", payload.FromAgent,
		"to", payload.ToAgent)

	// Forward message
	agentEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: evt.CorrelationID,
		AgentID:       targetAgent.ID(),
		Payload: core.AgentExecutePayload{
			Message:     payload.Content,
			MessageType: core.MessageTypeAgentMessage,
			Attachments: payload.Attachments,
			Agent: &core.AgentMetadata{
				FromAgent: payload.FromAgent,
			},
		},
	}

	select {
	case targetAgent.Inbox() <- agentEvent:
		k.logger.Debug("agent message delivered")

	case <-time.After(5 * time.Second):
		k.logger.Error("failed to deliver agent message - inbox full")

	case <-k.ctx.Done():
		return
	}
}

func (k *Kernel) sendDelegationError(evt core.Event, errorMsg string) {
	if evt.ReplyTo == nil {
		k.logger.Warn("cannot send delegation error - no reply channel")
		return
	}

	errorEvt := core.Event{
		Type:          core.DelegationResult,
		CorrelationID: evt.CorrelationID,
		Payload: core.DelegationResultPayload{
			Error: errorMsg,
		},
	}

	select {
	case evt.ReplyTo <- errorEvt:
		// Success
	case <-time.After(2 * time.Second):
		k.logger.Error("failed to send delegation error")
	case <-k.ctx.Done():
		return
	}
}
