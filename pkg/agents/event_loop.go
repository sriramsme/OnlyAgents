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

	phase := fmt.Sprintf("%s_execution", a.id)
	logger.Timing.StartPhase(evt.CorrelationID, phase)
	defer logger.Timing.EndPhase(evt.CorrelationID, phase)

	requestCtx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()

	a.logger.Debug("processing agent execute event",
		"correlation_id", evt.CorrelationID,
		"message_type", payload.MessageType,
		"message_length", len(payload.Message))

	var response string
	var err error
	if a.shouldStream(payload) {
		response, err = a.executeStream(requestCtx, payload, evt.CorrelationID)
	} else {
		response, err = a.execute(requestCtx, payload, evt.CorrelationID)
	}
	if err != nil {
		a.logger.Error("execute failed", "error", err, "correlation_id", evt.CorrelationID)
		a.sendError(payload.Channel, evt.ReplyTo, evt.CorrelationID, err)
		return
	}

	a.routeResponse(evt, payload, response)
}

// HELPER METHODS

// routeResponse dispatches the agent's response based on how the message arrived.
func (a *Agent) routeResponse(evt core.Event, payload core.AgentExecutePayload, response string) {
	switch payload.MessageType {
	case core.MessageTypeDelegation:
		if payload.Delegation != nil && payload.Delegation.SendDirectlyToUser {
			a.handleDirectDelegationResponse(payload, evt.CorrelationID, response)
		} else {
			a.sendDelegationResult(evt.ReplyTo, evt.CorrelationID, response)
		}
	case core.MessageTypeWorkflowTask:
		a.sendTaskResult(evt.ReplyTo, evt.CorrelationID, response)
	default:
		if evt.ReplyTo != nil {
			a.sendSyncResponse(evt.ReplyTo, evt.CorrelationID, response)
		} else {
			a.sendOutboundMessage(payload, evt.CorrelationID, response)
		}
	}
}

// handleDirectDelegationResponse sends the sub-agent's response directly to the
// user and injects it into the executive's history for continuity.
func (a *Agent) handleDirectDelegationResponse(payload core.AgentExecutePayload, correlationID, response string) {
	a.sendOutboundMessage(payload, correlationID, response)

	if payload.Delegation.FromAgentID == "" {
		return
	}
	attribution := fmt.Sprintf("[%s responded directly to user]\n\n%s", a.name, response)
	if err := a.cm.SaveAssistantMessageAt(
		a.ctx,
		payload.Channel.SessionID,
		payload.Delegation.FromAgentID,
		attribution,
		"",
		nil,
		payload.Delegation.DelegatedAt,
	); err != nil {
		a.logger.Warn("failed to inject response into executive history", "err", err)
	}
}
