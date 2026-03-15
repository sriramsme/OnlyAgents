package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// Inbox returns the channel kernel sends events to.
func (a *Agent) Inbox() chan<- core.Event {
	return a.inbox
}

func (a *Agent) processEvents() {
	defer a.wg.Done()
	sem := make(chan struct{}, a.maxConcurrency)

	for {
		select {
		case evt := <-a.inbox:
			sem <- struct{}{} // acquire slot (blocks if at max)
			go func(e core.Event) {
				defer func() { <-sem }() // release slot
				a.handleEvent(e)
			}(evt)
		case <-a.ctx.Done():
			a.logger.Info("event processor shutting down")
			return
		}
	}
}

// handleEvent is the event handler that processes delegation/workflow results
func (a *Agent) handleEvent(evt core.Event) {
	switch evt.Type {
	case core.AgentExecute:
		a.handleAgentExecute(evt)

	case core.DelegationResult:
		// This shouldn't arrive here - it goes to ReplyTo channel
		// But log for debugging
		a.logger.Debug("delegation result received (via event bus)",
			"correlation_id", evt.CorrelationID)

	case core.WorkflowCompleted:
		// This shouldn't arrive here - it goes to ReplyTo channel
		// But log for debugging
		a.logger.Debug("workflow completed (via event bus)",
			"correlation_id", evt.CorrelationID)

	default:
		a.logger.Warn("unhandled event type",
			"type", evt.Type,
			"correlation_id", evt.CorrelationID)
	}
}

// handleAgentExecute processes AgentExecute events
func (a *Agent) handleAgentExecute(evt core.Event) {
	payload, ok := evt.Payload.(core.AgentExecutePayload)
	if !ok {
		a.logger.Error("invalid AgentExecute payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload),
			"correlation_id", evt.CorrelationID)
		return
	}

	a.updateUI(payload.Message, 60)

	agentPhase := fmt.Sprintf("%s_execution", a.id)
	logger.Timing.StartPhase(evt.CorrelationID, agentPhase)

	requestCtx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()

	a.logger.Debug("processing agent execute event",
		"correlation_id", evt.CorrelationID,
		"message_type", payload.MessageType,
		"message_length", len(payload.Message))

	// Choose execute path based on whether this response goes to user directly
	var response string
	var err error
	if a.shouldStream(payload) {
		response, err = a.executeStream(requestCtx, payload, evt.CorrelationID)
	} else {
		response, err = a.execute(requestCtx, payload, evt.CorrelationID)
	}

	logger.Timing.EndPhase(evt.CorrelationID, agentPhase)

	if err != nil {
		a.logger.Error("execute failed",
			"error", err,
			"correlation_id", evt.CorrelationID)

		a.sendError(payload.Channel, evt.ReplyTo, evt.CorrelationID, err)
		return
	}

	// Determine how to respond based on message type
	messageType := payload.MessageType

	switch messageType {

	case core.MessageTypeDelegation:
		// Check if this is a delegation with direct user response
		sendDirectlyToUser := false
		if payload.Delegation != nil {
			sendDirectlyToUser = payload.Delegation.SendDirectlyToUser
		}
		if sendDirectlyToUser {
			a.logger.Debug("sending directly to user, delegation result received (via event bus)",
				"agent_id", a.id,
				"from_agent_id", payload.Delegation.FromAgentID,
				"send_directly_to_user", sendDirectlyToUser,
				"channel", payload.Channel)
			// Task was delegated to this agent - send result back
			a.sendOutboundMessage(payload, evt.CorrelationID, response)

			// Inject into executive's history immediately after the ack
			if payload.Delegation.FromAgentID != "" {
				attribution := fmt.Sprintf("[%s responded directly to user]\n\n%s", a.name, response)
				if err := a.cm.SaveAssistantMessageAt(
					a.ctx,
					payload.Channel.SessionID,
					payload.Delegation.FromAgentID, // saves under executive's agent_id
					attribution,
					"",
					nil,
					payload.Delegation.DelegatedAt,
				); err != nil {
					a.logger.Warn("failed to inject response into executive history", "err", err)
				}
			}
		} else {
			a.logger.Debug("delegation result received (via event bus)",
				"agent_id", a.id,
				"from_agent_id", payload.Delegation.FromAgentID,
				"delegated_at", payload.Delegation.DelegatedAt,
				"send_directly_to_user", sendDirectlyToUser,
				"channel", payload.Channel)
			// Task was delegated to this agent - send result back
			a.sendDelegationResult(evt.ReplyTo, evt.CorrelationID, response)
		}

	case core.MessageTypeWorkflowTask:
		// Task from workflow engine - send result back
		a.sendTaskResult(evt.ReplyTo, evt.CorrelationID, response)

	default:

		// Regular user message - send to channel
		if evt.ReplyTo != nil {
			// Sync response (HTTP)
			a.sendSyncResponse(evt.ReplyTo, evt.CorrelationID, response)
		} else {
			// Async response (channel)
			a.sendOutboundMessage(payload, evt.CorrelationID, response)
		}
	}
}
