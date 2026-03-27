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

	correlationID := evt.CorrelationID
	if correlationID == "" {
		correlationID = uuid.NewString()
	}

	// Direct reply routing — bypass executive if message targets a specific agent
	var targetAgentID string
	if payload.ReplyToPlatformMessageID != "" {
		if m, err := k.store.GetMessageByPlatformID(k.ctx, payload.ReplyToPlatformMessageID); err == nil {
			targetAgentID = m.AgentID
		}
	}
	if targetAgentID == "" {
		targetAgentID = k.agents.GetExecutive().ID()
	}

	target, err := k.agents.Get(targetAgentID)
	if err != nil {
		k.logger.Error("failed to get target agent", "target_agent_id", targetAgentID, "err", err)
		return
	}

	if err := k.cm.SaveUserMessage(k.ctx, payload.Channel.SessionID, target.ID(), payload.PlatformMessageID, payload.Content); err != nil {
		k.logger.Warn("failed to save user message", "err", err)
	}

	logger.Timing.StartPhase(correlationID, "end_to_end")
	logger.Timing.StartPhase(correlationID, "executive_routing")

	agentEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: correlationID,
		AgentID:       targetAgentID,
		Payload: core.AgentExecutePayload{
			Message:     payload.Content,
			MessageType: core.MessageTypeUser,
			Attachments: payload.Attachments,
			Channel: &core.ChannelMetadata{
				SessionID: payload.Channel.SessionID,
				ChatID:    payload.Channel.ChatID,
				Name:      payload.Channel.Name,
				UserID:    payload.Channel.UserID,
				Username:  payload.Channel.Username,
			},
		},
	}

	select {
	case target.Inbox() <- agentEvent:
		logger.Timing.EndPhase(correlationID, "executive_routing")
		k.logger.Debug("message routed",
			"correlation_id", correlationID,
			"agent_id", target.ID(),
			"direct", target.ID() != k.agents.GetExecutive().ID())
	case <-time.After(5 * time.Second):
		logger.Timing.EndPhase(correlationID, "executive_routing")
		k.logger.Error("agent inbox full - message dropped", "correlation_id", correlationID)
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
	var ch channels.Channel
	var err error

	// Fallback to active channel if not specified
	// In most cases, ChannelMetadata will be set by the caller
	if payload.Channel != nil {
		ch, err = k.channels.Get(payload.Channel.Name)
		if err != nil {
			logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
			k.logger.Error("channel not found",
				"channel", payload.Channel.Name,
				"correlation_id", evt.CorrelationID)

			ch = *k.channels.GetActive()
		}
	} else {
		ch = *k.channels.GetActive()
		payload.Channel, err = k.GetActiveChannelMetadata()
		if err != nil {
			logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
			k.logger.Error("failed to get active channel metadata",
				"error", err)
			return
		}
	}

	// Create timeout context for channel send
	ctx, cancel := context.WithTimeout(k.ctx, 10*time.Second)
	defer cancel()

	if payload.IsNotification {
		msgID, err := k.cm.SaveNotificationMessage(ctx, payload.Channel.SessionID, k.agents.GetExecutive().ID(), payload.Content)
		if err != nil {
			k.logger.Warn("failed to save notification message", "err", err)
		}
		payload.MessageID = msgID
	}

	outBoundMessage := channels.OutgoingMessage{
		Channel:     payload.Channel,
		Content:     payload.Content,
		Attachments: payload.Attachments,
		ReplyToID:   payload.ReplyToID,
		ParseMode:   payload.ParseMode,
		AgentID:     payload.AgentID,
		AgentName:   payload.AgentName,
	}
	result, err := ch.Send(ctx, outBoundMessage)
	if err != nil {
		logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
		k.logger.Error("failed to send outbound message",
			"channel", payload.Channel.Name,
			"chat_id", payload.Channel.ChatID,
			"correlation_id", evt.CorrelationID,
			"error", err)
	} else {
		if result.PlatformMessageID != "" {
			err := k.store.UpdateMessagePlatformID(ctx, payload.MessageID, result.PlatformMessageID)
			if err != nil {
				k.logger.Warn("failed to update message platform ID", "err", err)
			}
		}
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

	var ch channels.Channel
	var err error

	// Fallback to active channel if not specified
	// In most cases, ChannelMetadata will be set by the caller
	if payload.Channel != nil {
		ch, err = k.channels.Get(payload.Channel.Name)
		if err != nil {
			logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
			k.logger.Error("channel not found",
				"channel", payload.Channel.Name,
				"correlation_id", evt.CorrelationID)

			ch = *k.channels.GetActive()
		}
	} else {
		ch = *k.channels.GetActive()
		payload.Channel, err = k.GetActiveChannelMetadata()
		if err != nil {
			logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
			k.logger.Error("failed to get active channel metadata",
				"error", err)
			return
		}
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
