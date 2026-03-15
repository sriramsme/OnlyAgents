package agents

import (
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
)

func (a *Agent) emitUI(evt core.UIEvent) {
	if a.uiBus == nil {
		return
	}
	select {
	case a.uiBus <- evt:
	default: // drop — UI missing one frame is fine, blocking the agent is not
	}
}

func (a *Agent) updateUI(message string, maxLen int) {
	if len(message) > maxLen {
		message = message[:maxLen] + "…"
	}
	a.setState("active", message)
	a.activeSince = time.Now()

	a.emitUI(core.UIEvent{
		Type:      core.UIEventAgentActivated,
		Timestamp: time.Now(),
		AgentID:   a.id,
		Payload: core.AgentActivatedPayload{
			Task:  message,
			Model: a.llmClient.Model(),
		},
	})
	defer func() {
		a.setState("idle", "")
		a.emitUI(core.UIEvent{
			Type:      core.UIEventAgentIdle,
			Timestamp: time.Now(),
			AgentID:   a.id,
			Payload: core.AgentIdlePayload{
				DurationMs: time.Since(a.activeSince).Milliseconds(),
			},
		})
	}()
}
