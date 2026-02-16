package llm

import (
    "context"
    "fmt"
)

// Provider represents different LLM providers
type Provider string

const (
    ProviderAnthropic Provider = "anthropic"
    ProviderOpenAI    Provider = "openai"
	ProviderGemini    Provider = "google"
	ProviderLocal     Provider = "local"
)

// Client is the interface for LLM interactions
type Client interface {
    // Generate completion
    Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

    // Stream completion (for future)
    // Stream(ctx context.Context, req CompletionRequest) (<-chan string, error)

    // Provider info
    Provider() Provider
    Model() string
}

// CompletionRequest represents a request to the LLM
type CompletionRequest struct {
    // System prompt
    System string

    // Messages (conversation history)
    Messages []Message

    // Model parameters
    MaxTokens   int
    Temperature float64

    // Metadata
    Metadata map[string]string
}

// Message represents a single message in the conversation
type Message struct {
    Role    Role   `json:"role"`
    Content string `json:"content"`
}

// Role represents the message sender
type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleSystem    Role = "system"
)

// CompletionResponse represents the LLM's response
type CompletionResponse struct {
    Content      string
    StopReason   string
    InputTokens  int
    OutputTokens int
    Model        string
}

// Config holds LLM client configuration
type Config struct {
    Provider    Provider
    Model       string
    APIKey      string
    BaseURL     string
    MaxTokens   int
    Temperature float64
}

// NewClient creates a new LLM client based on provider
func NewClient(config Config) (Client, error) {
    switch config.Provider {
    case ProviderAnthropic:
        return NewAnthropicClient(config)
    case ProviderOpenAI:
        return nil, fmt.Errorf("OpenAI provider not yet implemented")
    default:
        return nil, fmt.Errorf("unsupported provider: %s", config.Provider)
    }
}
