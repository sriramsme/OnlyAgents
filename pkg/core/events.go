package core

// EventType identifies the type of event
type EventType string

const (
	// =====================================
	// User-Facing Events
	// =====================================

	// MessageReceived: Channel received a user message
	// Flow: Channel → Kernel → Executive Agent
	MessageReceived EventType = "message_received"

	// OutboundMessage: Agent has response for user
	// Flow: Agent → Kernel → Channel
	OutboundMessage EventType = "outbound_message"

	// =====================================
	// Agent Execution Events
	// =====================================

	// AgentExecute: Execute an agent with a message
	// Flow: Kernel → Agent
	AgentExecute EventType = "agent_execute"

	// =====================================
	// Tool Execution Events
	// =====================================

	// ToolCallRequest: Agent wants to execute a tool
	// Flow: Agent → Kernel → Skill
	ToolCallRequest EventType = "tool_call_request"

	// ToolCallResult: Tool execution result
	// Flow: Skill → Kernel → Agent
	ToolCallResult EventType = "tool_call_result"

	// =====================================
	// Agent-to-Agent Delegation Events
	// =====================================

	// AgentDelegate: Executive/Agent delegates task to another agent
	// Flow: Executive → Kernel → Specialized Agent
	AgentDelegate EventType = "agent_delegate"

	// DelegationResult: Agent finished delegated task
	// Flow: Specialized Agent → Kernel → Executive
	DelegationResult EventType = "delegation_result"

	// =====================================
	// Workflow Orchestration Events
	// =====================================

	// WorkflowSubmitted: Executive creates multi-task workflow
	// Flow: Executive → Kernel → Workflow Engine
	WorkflowSubmitted EventType = "workflow_submitted"

	// WorkflowCompleted: Workflow engine finished all tasks
	// Flow: Workflow Engine → Kernel → Executive
	WorkflowCompleted EventType = "workflow_completed"

	// TaskAssigned: Workflow engine assigns task to agent (internal)
	// Flow: Workflow Engine → Kernel → Agent
	// Note: NOT routed through executive - engine manages DAG internally
	TaskAssigned EventType = "task_assigned"

	// TaskCompleted: Task completed
	TaskCompleted EventType = "task_completed"

	// NewSession: Start a new session
	// Flow: Kernel → Agent
	NewSession EventType = "new_session"

	// =====================================
	// Future: Agent-to-Agent Communication
	// =====================================

	// AgentMessage: Direct agent-to-agent message (future)
	// Flow: Agent A → Kernel → Agent B
	AgentMessage EventType = "agent_message"
)

type MessageType string

const (
	MessageTypeUser              MessageType = "user"
	MessageTypeDelegation        MessageType = "delegation"
	MessageTypeWorkflowTask      MessageType = "workflow_task"
	MessageTypeAgentMessage      MessageType = "agent"
	MessageTypeWorkflowCompleted MessageType = "workflow_completed"
)

// Event represents a message passed through the event bus
type Event struct {
	Type          EventType    `json:"type"`
	CorrelationID string       `json:"correlation_id"` // For request tracing
	AgentID       string       `json:"agent_id"`       // Target agent
	Payload       interface{}  `json:"payload"`
	ReplyTo       chan<- Event `json:"-"` // For sync responses (not serialized)
}

// =====================================
// Event Payloads
// =====================================

// MessageReceivedPayload: User message from channel
type MessageReceivedPayload struct {
	ChannelName string            `json:"channel_name"`
	ChatID      string            `json:"chat_id"`
	UserID      string            `json:"user_id"`
	Username    string            `json:"username"`
	Content     string            `json:"content"`
	Metadata    map[string]string `json:"metadata"`
}

// OutboundMessagePayload: Response to send to channel
type OutboundMessagePayload struct {
	ChannelName string `json:"channel_name"`
	ChatID      string `json:"chat_id"`
	Content     string `json:"content"`
	ReplyToID   string `json:"reply_to_id,omitempty"`
	ParseMode   string `json:"parse_mode,omitempty"`
}

// AgentExecutePayload: Execute agent with message
type AgentExecutePayload struct {
	Message     string              `json:"user_message"`
	MessageType MessageType         `json:"message_type"`
	Channel     *ChannelMetadata    `json:"channel,omitempty"`
	Delegation  *DelegationMetadata `json:"delegation,omitempty"`
	Workflow    *WorkflowMetadata   `json:"workflow,omitempty"`
	Agent       *AgentMetadata      `json:"agent,omitempty"`
}

type ChannelMetadata struct {
	ChatID   string `json:"chat_id"`
	Name     string `json:"name"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type DelegationMetadata struct {
	DelegationID       string `json:"delegation_id"`
	SendDirectlyToUser bool   `json:"send_directly_to_user"`
}

type WorkflowMetadata struct {
	WorkflowID string `json:"workflow_id"`
	TaskID     string `json:"task_id"`
	TaskName   string `json:"task_name"`
}

type AgentMetadata struct {
	FromAgent string `json:"from_agent"`
}

// AgentDelegatePayload: Delegate task to another agent
type AgentDelegatePayload struct {
	DelegationID       string         `json:"delegation_id"` // Unique delegation ID
	AgentID            string         `json:"agent_id"`      // Target agent ID (specified by executive)
	Task               string         `json:"task"`          // Task description
	Capabilities       []Capability   `json:"capabilities"`  // Required capabilities (for validation)
	Context            map[string]any `json:"context,omitempty"`
	SendDirectlyToUser bool           `json:"send_directly_to_user"`
	Timeout            int            `json:"timeout,omitempty"` // Seconds

	// In case is sending directly to user, sub-agent needs chatID, channelName etc
	Channel *ChannelMetadata `json:"channel,omitempty"`
}

// DelegationResultPayload: Result of delegated task
type DelegationResultPayload struct {
	DelegationID string `json:"delegation_id"`
	Result       any    `json:"result,omitempty"`
	Error        string `json:"error,omitempty"`
}

// AgentMessagePayload: Direct agent-to-agent message (future)
type AgentMessagePayload struct {
	FromAgent string         `json:"from_agent"`
	ToAgent   string         `json:"to_agent"`
	Content   string         `json:"content"`
	Context   map[string]any `json:"context,omitempty"`
}

// ErrorPayload: Error response
type ErrorPayload struct {
	Error string `json:"error"`
}
