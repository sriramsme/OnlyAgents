package llm

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// Provider represents different LLM providers
type Provider string

// Role represents the message sender
type Role string

type StopReason string

const (
	StopReasonUnknown StopReason = "unknown"
	StopReasonEnd     StopReason = "end"
	StopReasonLength  StopReason = "length"
	StopReasonTool    StopReason = "tool"
	StopReasonContent StopReason = "content_filter"
)

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGemini    Provider = "gemini"
)

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// Client is the interface for LLM interactions
type Client interface {
	// Chat sends a chat completion request
	Chat(ctx context.Context, req *Request) (*Response, error)
	ChatStream(ctx context.Context, req *Request) <-chan StreamChunk
	// Provider returns the provider name
	Provider() Provider

	// Model returns the model identifier
	Model() string

	ContextWindow() int
}

// Request represents a chat completion request
type Request struct {
	// Messages in the conversation
	Messages []Message

	// Tools available for the model to call
	Tools []tools.ToolDef

	// Model parameters (optional - uses client defaults if zero)
	MaxTokens   int
	Temperature float64

	// Metadata for tracking and debugging
	Metadata map[string]string
}

// Message represents a single message in the conversation
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content,omitempty"`

	// Parts carries multimodal content (text + images + documents).
	// When Parts is non-nil, providers use it instead of Content.
	// Text-only messages continue to use Content — no migration needed.
	Parts []ContentPart `json:"parts,omitempty"`

	// For extended thinking models (Sonnet 4.5, Opus 4.6)
	ReasoningContent string `json:"reasoning_content,omitempty"`

	// For assistant messages with tool calls
	ToolCalls []tools.ToolCall `json:"tool_calls,omitempty"`

	// For tool result messages
	ToolCallID string `json:"tool_call_id,omitempty"`
	Name       string `json:"name,omitempty"` // tool name

	// Required for Gemini 3 stateful multi-turn tool use
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

// StreamChunk represents a chunk of streaming response
type StreamChunk struct {
	Content   string
	ToolCalls []tools.ToolCall
	Done      bool
	Error     error
	Usage     Usage
}

// Response represents the LLM's response
type Response struct {
	Content          string           // Final text response
	ReasoningContent string           // Extended thinking/reasoning
	ToolCalls        []tools.ToolCall // Tool calls (if any)
	StopReason       StopReason
	Usage            Usage
	Model            string
}

// Usage represents token usage information
type Usage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CachedTokens    int `json:"cached_tokens,omitempty"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	TotalTokens     int `json:"total_tokens"`
}

// ProviderConfig is the configuration passed to provider constructors
type ProviderConfig struct {
	Model   string
	APIKey  string
	BaseURL string
	Options *Options
}

// ProviderConstructor creates a new provider client
type ProviderConstructor func(cfg ProviderConfig) (Client, error)

// ProviderRegistration contains metadata about a provider
type ProviderRegistration struct {
	Models      []string
	EnvKey      string // Environment variable for API key
	Constructor ProviderConstructor
}
