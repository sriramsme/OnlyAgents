package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/media"
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
		"message_length", len(payload.Message),
		"attachments", len(payload.Attachments))

	var response *llm.Response
	var err error
	var producedFiles []*media.Attachment

	if a.shouldStream(payload) {
		response, producedFiles, err = a.executeStream(requestCtx, payload, evt.CorrelationID)
	} else {
		response, producedFiles, err = a.execute(requestCtx, payload, evt.CorrelationID)
	}
	if err != nil {
		a.logger.Error("execute failed", "error", err, "correlation_id", evt.CorrelationID)
		a.sendError(payload.Channel, evt.ReplyTo, evt.CorrelationID, err)
		return
	}

	a.routeResponse(evt, payload, response, producedFiles)
}

// HELPER METHODS

// routeResponse dispatches the agent's response based on how the message arrived.
func (a *Agent) routeResponse(
	evt core.Event,
	payload core.AgentExecutePayload,
	response *llm.Response,
	attachments []*media.Attachment,
) {
	switch payload.MessageType {

	case core.MessageTypeWorkflowTask:
		a.sendTaskResult(evt.ReplyTo, evt.CorrelationID, response)

	case core.MessageTypeDelegation:
		if payload.Delegation != nil && payload.Delegation.SendDirectlyToUser {
			a.sendOutboundMessage(OutboundMessageInput{
				Channel:       payload.Channel,
				Response:      response,
				Attachments:   attachments,
				CorrelationID: evt.CorrelationID,
			})
		} else {
			a.sendDelegationResult(evt.ReplyTo, evt.CorrelationID, response.Content)
		}
	default:
		if evt.ReplyTo != nil {
			a.sendSyncResponse(evt.ReplyTo, evt.CorrelationID, response.Content)
		} else {
			a.sendOutboundMessage(OutboundMessageInput{
				Channel:       payload.Channel,
				Response:      response,
				Attachments:   attachments,
				CorrelationID: evt.CorrelationID,
			})
		}
	}
}
