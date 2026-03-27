package agents

import (
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
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

type OutboundMessageInput struct {
	Channel       *core.ChannelMetadata
	Response      *llm.Response
	Attachments   []*media.Attachment
	CorrelationID string
}

func (a *Agent) sendOutboundMessage(input OutboundMessageInput) {
	msgID, err := a.cm.SaveAssistantMessage(
		a.ctx,
		input.Channel.SessionID,
		a.id,
		input.Response.Content,
		input.Response.ReasoningContent,
		input.Response.ToolCalls, // will be empty since outbound message is sendt after processing all tool calls
	)
	if err != nil {
		a.logger.Warn("failed to inject response into executive history", "err", err)
	}

	evt := core.Event{
		Type:          core.OutboundMessage,
		CorrelationID: input.CorrelationID,
		Payload: core.OutboundMessagePayload{
			MessageID:   msgID,
			Channel:     input.Channel,
			Content:     input.Response.Content,
			Attachments: input.Attachments,
			AgentID:     a.id,
			AgentName:   a.name,
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
		a.sendOutboundMessage(OutboundMessageInput{
			Channel: channel,
			Response: &llm.Response{
				Content: "Sorry, I ran into an issue processing your request. Please try again.",
			},
			CorrelationID: correlationID,
		})
	}
}
