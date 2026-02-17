package a2a

import (
	"fmt"
	"time"
)

// Message represents an agent-to-agent message
type Message struct {
	// Protocol
	ProtocolVersion string `json:"protocol_version"`

	// Routing
	FromAgent string `json:"from_agent"`
	ToAgent   string `json:"to_agent"`

	// Message
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Timestamp      time.Time `json:"timestamp"`

	// Action
	Action  string                 `json:"action"`
	Payload map[string]interface{} `json:"payload"`

	// Security
	Signature []byte `json:"signature"`
	PublicKey []byte `json:"public_key"`

	// Metadata
	TTL      int    `json:"ttl"`
	Priority string `json:"priority"`
}

// NewMessage creates a new message
func NewMessage(from, to, action string) Message {
	return Message{
		ProtocolVersion: "a2a/1.0",
		FromAgent:       from,
		ToAgent:         to,
		ID:              generateMessageID(),
		ConversationID:  generateConversationID(),
		Timestamp:       time.Now(),
		Action:          action,
		Payload:         make(map[string]interface{}),
		TTL:             300,
		Priority:        "normal",
	}
}

func generateMessageID() string {
	// TODO: Implement proper ID generation
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

func generateConversationID() string {
	// TODO: Implement proper conversation ID generation
	return fmt.Sprintf("conv_%d", time.Now().UnixNano())
}
