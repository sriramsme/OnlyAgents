package kernel

import (
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// GetOrCreate — used by all channels on every message
func (k *Kernel) handleSessionGet(evt core.Event) {
	p := evt.Payload.(core.SessionGetPayload)
	sessionID, err := k.store.GetOrCreateSession(k.ctx, p.Channel, p.AgentID)
	if err != nil {
		k.logger.Error("session.get failed", "err", err)
		if evt.ReplyTo != nil {
			evt.ReplyTo <- core.Event{Payload: ""}
		}
		return
	}
	if evt.ReplyTo != nil {
		evt.ReplyTo <- core.Event{Payload: sessionID}
	}
}

// handleNewSession ends the current conversation and starts a fresh one.
// Triggered by a NewSession event, typically from a /newsession channel command.
func (k *Kernel) handleSessionNew(evt core.Event) {
	payload, ok := evt.Payload.(core.SessionNewPayload)
	if !ok {
		k.logger.Error("invalid NewSession payload")
		return
	}
	// End existing if any
	if existing, err := k.store.GetOrCreateSession(k.ctx, payload.Channel, payload.AgentID); err == nil {
		err := k.cm.EndSession(k.ctx, existing)
		if err != nil {
			k.logger.Error("failed to end existing session",
				"channel", payload.Channel,
				"session_id", existing,
				"err", err)
		}
	}

	sessionID, err := k.cm.NewSession(k.ctx, payload.Channel, payload.AgentID)
	if err != nil {
		k.logger.Error("failed to start new session",
			"channel", payload.Channel,
			"err", err)
		// Reply with empty string so caller doesn't hang
		if evt.ReplyTo != nil {
			evt.ReplyTo <- core.Event{Payload: ""}
		}
		return
	}

	k.logger.Info("new session started",
		"channel", payload.Channel,
		"session_id", sessionID)

	if evt.ReplyTo != nil {
		evt.ReplyTo <- core.Event{Payload: sessionID}
	}
}

func (k *Kernel) handleSessionEnd(evt core.Event) {
	payload, ok := evt.Payload.(core.SessionEndPayload)
	if !ok {
		k.logger.Error("invalid SessionEnd payload")
		return
	}

	if err := k.cm.EndSession(k.ctx, payload.SessionID); err != nil {
		k.logger.Warn("failed to end session",
			"session_id", payload.SessionID,
			"err", err)
		return
	}

	k.logger.Info("session ended", "session_id", payload.SessionID)
	// No ReplyTo needed — fire and forget
}
