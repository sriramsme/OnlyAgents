package llm

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
)

// Provider represents different LLM providers
type Provider string

// Client is the interface for LLM interactions
type Client interface {
	// Chat sends a chat completion request
	Chat(ctx context.Context, req *Request) (*Response, error)

	// Provider returns the provider name
	Provider() Provider

	// Model returns the model identifier
	Model() string
}

// Request represents a chat completion request
type Request struct {
	// Messages in the conversation
	Messages []Message

	// Tools available for the model to call
	Tools []ToolDef

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

	// For extended thinking models (Sonnet 4.5, Opus 4.6)
	ReasoningContent string `json:"reasoning_content,omitempty"`

	// For assistant messages with tool calls
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// For tool result messages
	ToolCallID string `json:"tool_call_id,omitempty"`
	Name       string `json:"name,omitempty"` // tool name
}

// Role represents the message sender
type Role string

// ToolCall represents a tool invocation by the model
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall represents a function call
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolDef defines a tool for the LLM (function calling)
type ToolDef struct {
	Type     string      `json:"type"` // "function"
	Function FunctionDef `json:"function"`
}

// FunctionDef defines a function that the model can call
type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// Response represents the LLM's response
type Response struct {
	Content          string     // Final text response
	ReasoningContent string     // Extended thinking/reasoning
	ToolCalls        []ToolCall // Tool calls (if any)
	StopReason       string
	Usage            Usage
	Model            string
}

// Usage represents token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ProviderConfig is the configuration passed to provider constructors
type ProviderConfig struct {
	Model       string
	Vault       vault.Vault
	KeyPath     string //Path to the API key
	BaseURL     string
	MaxTokens   int
	Temperature float64
	Metadata    map[string]string
}

// ProviderConstructor creates a new provider client
type ProviderConstructor func(cfg ProviderConfig) (Client, error)

// ProviderRegistration contains metadata about a provider
type ProviderRegistration struct {
	Models      []string
	EnvKey      string // Environment variable for API key
	Constructor ProviderConstructor
}
