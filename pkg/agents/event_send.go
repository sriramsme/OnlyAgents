package agents

import (
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/media"
)

// Helper methods for sending different types of responses

// safeReply sends to a reply channel with timeout and context checks
func (a *Agent) safeReply(replyCh chan<- core.Event, evt core.Event, description string) {
	select {
	case replyCh <- evt:
		// Success
	case <-time.After(5 * time.Second):
		a.logger.Error("failed to send reply - timeout",
			"description", description,
			"correlation_id", evt.CorrelationID)
	case <-a.ctx.Done():
		a.logger.Info("failed to send reply - agent shutting down",
			"description", description,
			"correlation_id", evt.CorrelationID)
	}
}

// safeSend sends to outbox with timeout and context checks
func (a *Agent) safeSend(evt core.Event, description string) {
	select {
	case a.outbox <- evt:
		// Success
	case <-time.After(5 * time.Second):
		a.logger.Error("failed to send to outbox - timeout",
			"description", description,
			"event_type", evt.Type,
			"correlation_id", evt.CorrelationID)
	case <-a.ctx.Done():
		a.logger.Info("failed to send to outbox - agent shutting down",
			"description", description,
			"event_type", evt.Type,
			"correlation_id", evt.CorrelationID)
	}
}

func (a *Agent) sendDelegationResult(replyCh chan<- core.Event, correlationID string, result any) {
	if replyCh == nil {
		return
	}
	evt := core.Event{
		Type:          core.DelegationResult,
		CorrelationID: correlationID,
		Payload: core.DelegationResultPayload{
			Result: result,
		},
	}
	a.safeReply(replyCh, evt, "delegation result")
}

func (a *Agent) sendTaskResult(replyCh chan<- core.Event, correlationID string, result any) {
	if replyCh == nil {
		return
	}

	// Task result goes back to workflow engine
	// For now, use same structure as delegation result
	evt := core.Event{
		Type:          core.DelegationResult, // Workflow engine expects this
		CorrelationID: correlationID,
		Payload: core.DelegationResultPayload{
			Result: result,
		},
	}

	a.safeReply(replyCh, evt, "task result")
}

func (a *Agent) sendSyncResponse(replyCh chan<- core.Event, correlationID string, response string) {
	if replyCh == nil {
		return
	}

	evt := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: correlationID,
		Payload:       response,
	}

	a.safeReply(replyCh, evt, "sync response")
}

func (a *Agent) sendOutboundMessage(
	payload core.AgentExecutePayload,
	correlationID string,
	response string,
	attachments []*media.Attachment,
) {
	evt := core.Event{
		Type:          core.OutboundMessage,
		CorrelationID: correlationID,
		Payload: core.OutboundMessagePayload{
			Channel:     payload.Channel,
			Content:     response,
			Attachments: attachments,
		},
	}

	a.safeSend(evt, "outbound message")
}

func (a *Agent) sendError(channel *core.ChannelMetadata, replyCh chan<- core.Event, correlationID string, err error) {
	if replyCh != nil {

		evt := core.Event{
			Type:          core.DelegationResult,
			CorrelationID: correlationID,
			Payload: core.DelegationResultPayload{
				Error: err.Error(),
			},
		}
		a.safeReply(replyCh, evt, "error response")
	}
	// Don't leave user hanging
	if channel != nil {
		a.safeSend(core.Event{
			Type:          core.OutboundMessage,
			CorrelationID: correlationID,
			AgentID:       a.ID(),
			Payload: core.OutboundMessagePayload{
				Channel: channel,
				Content: "Sorry, I ran into an issue processing your request. Please try again.",
			},
		}, "execute error fallback")
	}
}
