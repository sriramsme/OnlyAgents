package core

import "time"

// UIBus is the channel carrying UIEvents from agents/kernel to SSE clients.
// A nil UIBus means headless mode — all emit calls are zero-cost no-ops.
type UIBus = chan UIEvent

// UIBusBuffer is the default buffer size for the UIBus channel.
const UIBusBuffer = 256

// UIEventType identifies the kind of UI event.
type UIEventType string

const (
	UIEventAgentActivated UIEventType = "agent.activated"
	UIEventAgentIdle      UIEventType = "agent.idle"
	UIEventAgentError     UIEventType = "agent.error"
	UIEventToolCalled     UIEventType = "tool.called"
	UIEventToolResult     UIEventType = "tool.result"
	UIEventDelegation     UIEventType = "delegation"
	UIEventHeartbeat      UIEventType = "heartbeat"
	UIEventSnapshotAgent  UIEventType = "snapshot.agent" // initial state dump on SSE connect
)

// UIEvent is the payload sent over the UIBus and forwarded to SSE clients.
type UIEvent struct {
	Type      UIEventType `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	AgentID   string      `json:"agent_id,omitempty"`
	Payload   any         `json:"payload,omitempty"`
}

// ─── Payload types ────────────────────────────────────────────────────────────

type AgentActivatedPayload struct {
	Task  string `json:"task"`
	Model string `json:"model"`
}

type AgentIdlePayload struct {
	DurationMs int64 `json:"duration_ms"`
}

type AgentErrorPayload struct {
	Error string `json:"error"`
}

type ToolCalledPayload struct {
	ToolName string `json:"tool_name"`
	Input    string `json:"input"`
}

type ToolResultPayload struct {
	ToolName   string `json:"tool_name"`
	Success    bool   `json:"success"`
	DurationMs int64  `json:"duration_ms"`
}

type DelegationPayload struct {
	FromAgent string `json:"from_agent"`
	ToAgent   string `json:"to_agent"`
	Task      string `json:"task"`
}

// ─── AgentStatus ──────────────────────────────────────────────────────────────

type AgentState string

const (
	AgentStateIdle   AgentState = "idle"   // nothing in inbox
	AgentStateActive AgentState = "active" // something in inbox
	AgentStateError  AgentState = "error"  // issue with agent
	AgentStateBusy   AgentState = "busy"   // inbox full
)

// AgentStatus is the runtime snapshot of an agent.
// Used by KernelReader.Agents() and the snapshot.agent event on SSE connect.
type AgentStatus struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	State       AgentState `json:"state"`
	CurrentTask string     `json:"current_task,omitempty"` // truncated, empty when idle
	LastActive  time.Time  `json:"last_active"`
	Model       string     `json:"model"`
	IsExecutive bool       `json:"is_executive"`
}
