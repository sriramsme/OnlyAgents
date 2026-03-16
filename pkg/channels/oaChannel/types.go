package oaChannel

import (
	"time"

	"github.com/sriramsme/OnlyAgents/internal/config"
)

// Config holds OAChannel configuration.
type Config struct {
	Channel      config.Channel
	AllowOrigins []string `yaml:"allow_origins,omitempty"` // lock down in production
}

// WSMessageType identifies inbound and outbound WebSocket message types.
type WSMessageType string

const (
	// ── UI → Server ──────────────────────────────────────────────────────────
	WSMsgChat       WSMessageType = "chat"        // text message to agent
	WSMsgVoiceChunk WSMessageType = "voice.chunk" // raw audio chunk (base64)
	WSMsgVoiceEnd   WSMessageType = "voice.end"   // user finished speaking
	WSMsgNewSession WSMessageType = "session.new" // start a fresh session
	WSMsgPing       WSMessageType = "ping"

	// ── Server → UI (chat / voice) ───────────────────────────────────────────
	WSMsgAgentText     WSMessageType = "agent.text"     // text reply (final or streaming token)
	WSMsgAgentVoice    WSMessageType = "agent.voice"    // audio chunk from agent
	WSMsgAgentThinking WSMessageType = "agent.thinking" // agent is processing
	WSMsgNotification  WSMessageType = "notification"   // proactive system alert

	// ── Server → UI (war room — mirrors core.UIEvent types) ──────────────────
	WSMsgAgentActivated WSMessageType = "agent.activated"
	WSMsgAgentIdle      WSMessageType = "agent.idle"
	WSMsgAgentError     WSMessageType = "agent.error"
	WSMsgToolCalled     WSMessageType = "tool.called"
	WSMsgToolResult     WSMessageType = "tool.result"
	WSMsgDelegation     WSMessageType = "delegation"
	WSMsgSnapshot       WSMessageType = "snapshot"

	WSMsgPong WSMessageType = "pong"
)

// WSMessage is the envelope for every WebSocket frame.
type WSMessage struct {
	Type      WSMessageType `json:"type"`
	SessionID string        `json:"session_id,omitempty"`
	AgentID   string        `json:"agent_id,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
	Payload   any           `json:"payload,omitempty"`
}

// ── Inbound payload types ─────────────────────────────────────────────────────

// ChatPayload is the payload for WSMsgChat.
type ChatPayload struct {
	Message string `json:"message"`
	AgentID string `json:"agent_id,omitempty"` // overrides connection-level agent_id
}

// VoiceChunkPayload is the payload for WSMsgVoiceChunk.
type VoiceChunkPayload struct {
	Data       string `json:"data"`     // base64 encoded audio
	Encoding   string `json:"encoding"` // "opus" | "pcm16"
	SampleRate int    `json:"sample_rate"`
}

// ── Outbound payload types ────────────────────────────────────────────────────

// AgentTextPayload is the payload for WSMsgAgentText.
// IsFinal=false means a streaming token; IsFinal=true means the turn is complete.
type AgentTextPayload struct {
	Content string `json:"content"`
	AgentID string `json:"agent_id"`
	IsFinal bool   `json:"is_final"`
}

// AgentVoicePayload is the payload for WSMsgAgentVoice.
type AgentVoicePayload struct {
	Data     string `json:"data"`     // base64 encoded audio chunk
	Encoding string `json:"encoding"` // "opus" | "pcm16"
	IsFinal  bool   `json:"is_final"`
}

// NotificationPayload is the payload for WSMsgNotification.
type NotificationPayload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Severity string `json:"severity"` // "info" | "warning" | "alert"
	Link     string `json:"link,omitempty"`
}

// NewSessionPayload is sent server→UI after session.new is processed.
type NewSessionPayload struct {
	SessionID string `json:"session_id"`
}
