package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// handleMessageReceived: ALL user messages go to executive first
func (k *Kernel) handleMessageReceived(evt core.Event) {
	payload, ok := evt.Payload.(core.MessageReceivedPayload)
	if !ok {
		k.logger.Error("invalid MessageReceived payload")
		return
	}

	// Get executive agent (entry point for all user messages)
	executive := k.agents.GetExecutive()

	correlationID := evt.CorrelationID
	if correlationID == "" {
		correlationID = uuid.NewString()
	}
	logger.Timing.StartPhase(correlationID, "end_to_end")
	logger.Timing.StartPhase(correlationID, "executive_routing")

	// Create agent execution event
	agentEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: correlationID,
		AgentID:       executive.ID(),
		Payload: core.AgentExecutePayload{
			Message:     payload.Content,
			MessageType: core.MessageTypeUser,
			Channel: &core.ChannelMetadata{
				SessionID: payload.Channel.SessionID,
				ChatID:    payload.Channel.ChatID,
				Name:      payload.Channel.Name,
				UserID:    payload.Channel.UserID,
				Username:  payload.Channel.Username,
			},
		},
	}

	// Send to executive
	select {
	case executive.Inbox() <- agentEvent:
		logger.Timing.EndPhase(correlationID, "executive_routing")
		k.logger.Debug("message routed to executive",
			"correlation_id", correlationID,
			"executive_id", executive.ID())

	case <-time.After(5 * time.Second):
		logger.Timing.EndPhase(correlationID, "executive_routing")
		k.logger.Error("executive inbox full - message dropped",
			"correlation_id", correlationID)
		// TODO: Send error response back to channel

	case <-k.ctx.Done():
		logger.Timing.EndPhase(correlationID, "executive_routing")
		k.logger.Info("shutdown in progress - message not delivered")
	}
}

// handleOutboundMessage: agent has a response, send it via the appropriate channel
func (k *Kernel) handleOutboundMessage(evt core.Event) {
	payload, ok := evt.Payload.(core.OutboundMessagePayload)
	if !ok {
		k.logger.Error("invalid OutboundMessage payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload))
		return
	}

	logger.Timing.StartPhase(evt.CorrelationID, "outbound_send")
	ch, err := k.channels.Get(payload.Channel.Name)
	if err != nil {
		logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
		k.logger.Error("channel not found",
			"channel", payload.Channel.Name,
			"correlation_id", evt.CorrelationID)
		return
	}
	// Create timeout context for channel send
	ctx, cancel := context.WithTimeout(k.ctx, 10*time.Second)
	defer cancel()

	if err := ch.Send(ctx, channels.OutgoingMessage{
		Channel:   payload.Channel,
		Content:   payload.Content,
		ReplyToID: payload.ReplyToID,
		ParseMode: payload.ParseMode,
	}); err != nil {
		logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
		k.logger.Error("failed to send outbound message",
			"channel", payload.Channel.Name,
			"correlation_id", evt.CorrelationID,
			"error", err)
	} else {
		logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
		k.logger.Debug("outbound message sent",
			"channel", payload.Channel.Name,
			"correlation_id", evt.CorrelationID)
	}

	logger.Timing.EndPhase(evt.CorrelationID, "end_to_end")
	logger.Timing.LogSummary(evt.CorrelationID)
}

// handleOutboundToken: Agent has a response, send it via the appropriate channel
func (k *Kernel) handleOutboundToken(evt core.Event) {
	payload, ok := evt.Payload.(core.OutboundTokenPayload)
	if !ok {
		k.logger.Error("invalid AgentToken payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload))
		return
	}

	k.logger.Debug("sending token to channel",
		"channel", payload.Channel.Name,
		"token", payload.Token,
		"message", payload.AccumulatedContent,
		"correlation_id", evt.CorrelationID)

	ch, err := k.channels.Get(payload.Channel.Name)
	if err != nil {
		k.logger.Error("channel not found for token",
			"channel", payload.Channel.Name,
			"correlation_id", evt.CorrelationID)
		return
	}

	streamer, ok := ch.(channels.TokenStreamer)
	if !ok {
		// Channel doesn't support streaming — silently skip,
		// final response still arrives via Send()
		k.logger.Debug("channel doesn't support streaming",
			"channel", payload.Channel.Name,
			"token", payload.Token,
			"message", payload.AccumulatedContent,
			"correlation_id", evt.CorrelationID)
		return
	}

	ctx, cancel := context.WithTimeout(k.ctx, 5*time.Second)
	defer cancel()

	if err := streamer.SendToken(ctx, payload.Channel, payload.Token, payload.AccumulatedContent); err != nil {
		k.logger.Debug("send token failed",
			"channel", payload.Channel.Name,
			"error", err)
	}
	k.logger.Debug("sent token",
		"channel", payload.Channel.Name,
		"token", payload.Token,
		"message", payload.AccumulatedContent,
		"correlation_id", evt.CorrelationID)
}
