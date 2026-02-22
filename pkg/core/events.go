package core

// EventType defines all events flowing through the kernel bus
type EventType string

const (
	// Inbound: channel → kernel → agent
	MessageReceived EventType = "message.received"

	// Kernel → agent: agent should start executing
	AgentExecute EventType = "agent.execute"

	// Agent → kernel: LLM wants to call a tool
	ToolCallRequest EventType = "tool.call.request"

	// Kernel → agent: tool result ready, resume LLM loop
	ToolCallResult EventType = "tool.call.result"

	// Skill → kernel: skill needs a sub-agent to do something
	AgentRequest EventType = "agent.request"

	// Agent/skill → kernel → channel: send a message out
	OutboundMessage EventType = "message.outbound"
)

// Event is the universal message on the kernel bus.
// CorrelationID ties a request to its response across hops.
type Event struct {
	Type          EventType
	CorrelationID string     // e.g. request ID, ties ToolCallRequest → ToolCallResult
	AgentID       string     // which agent this event is for/from
	ReplyTo       chan Event `json:"-"` // for sync request/response (not serialized)
	Payload       any
}

// --- Payload types ---

// MessageReceivedPayload: inbound message from a channel (telegram, discord, etc.)
type MessageReceivedPayload struct {
	ChannelName string
	ChatID      string
	UserID      string
	Username    string
	Content     string
	Metadata    map[string]string
}

// AgentExecutePayload: tells an agent to handle a user message
type AgentExecutePayload struct {
	UserMessage string
	ChatID      string // so agent knows where to reply
	Metadata    map[string]string
}

// ToolCallRequestPayload: agent asking kernel to execute a tool/skill
type ToolCallRequestPayload struct {
	ToolCallID string // from LLM response, must be echoed back
	SkillName  string // e.g. "email", "calendar"
	ToolName   string // specific function e.g. "send_email"
	Params     map[string]any
}

// ToolCallResultPayload: kernel returning skill result to agent
type ToolCallResultPayload struct {
	ToolCallID string
	ToolName   string
	Result     any
	Error      string // non-empty if skill failed
}

// AgentRequestPayload: skill needs a sub-agent (e.g. to draft something)
type AgentRequestPayload struct {
	RequestingSkill string
	Task            string
	Context         map[string]any
}

// OutboundMessagePayload: send a response back to a channel
type OutboundMessagePayload struct {
	ChannelName string
	ChatID      string
	Content     string
	ReplyToID   string
	ParseMode   string
}

// ErrorPayload: send an error response back to a channel
type ErrorPayload struct {
	Error string
}

// Telegram
//   → Event{MessageReceived}
//   → Kernel
//   → Event{AgentExecute} → Agent
//                            Agent builds messages, gets tools from soul/skills
//                            calls LLM
//                            LLM returns ToolCall
//   → Event{ToolCallRequest, payload: {skill: "email", fn: "send", args: ...}}
//   → Kernel
//   → Skill.Execute()
//       skill needs a sub-agent?
//   → Event{AgentRequest}
//   → Kernel spins up/routes to agent
//   → ... (same flow recursively)
//       skill gets result
//   → Event{ToolCallResult}
//   → Kernel
//   → Agent (resumes with tool result, calls LLM again)
//   → Event{OutboundMessage}
//   → Kernel
//   → GmailConnector.Send()
