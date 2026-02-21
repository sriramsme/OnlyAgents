package llm

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// MockClient is a mock LLM client for testing
type MockClient struct {
	responses        []string
	callCount        int
	capturedReqs     []*llm.Request
	shouldError      bool
	errorMsg         string
	simulateTools    bool
	toolCallResult   string
	simulateThinking bool
	thinkingContent  string
}

// MockClientOption configures the mock client
type MockClientOption func(*MockClient)

// WithResponses sets predefined responses
func WithResponses(responses ...string) MockClientOption {
	return func(m *MockClient) {
		m.responses = responses
	}
}

// WithError makes the client return an error
func WithError(msg string) MockClientOption {
	return func(m *MockClient) {
		m.shouldError = true
		m.errorMsg = msg
	}
}

// WithToolCall makes the client simulate a tool call
func WithToolCall(toolName string, args string) MockClientOption {
	return func(m *MockClient) {
		m.simulateTools = true
		m.toolCallResult = fmt.Sprintf(`{"tool":"%s","args":%s}`, toolName, args)
	}
}

// WithThinking makes the client simulate extended thinking
func WithThinking(thinking string) MockClientOption {
	return func(m *MockClient) {
		m.simulateThinking = true
		m.thinkingContent = thinking
	}
}

// NewMockClient creates a mock client with options
func NewMockClient(opts ...MockClientOption) *MockClient {
	m := &MockClient{
		responses:    []string{"Mock response"},
		capturedReqs: make([]*llm.Request, 0),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// MockClientWithThinking creates a mock client that simulates extended thinking
func MockClientWithThinking(thinking, response string) *MockClient {
	return NewMockClient(
		WithResponses(response),
		WithThinking(thinking),
	)
}

// Updated Chat to support thinking
func (m *MockClient) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	// Capture request for testing assertions
	m.capturedReqs = append(m.capturedReqs, req)

	// Simulate error if configured
	if m.shouldError {
		return nil, fmt.Errorf("mock error: %s", m.errorMsg)
	}

	// Get response content
	content := "Mock response"
	if m.callCount < len(m.responses) {
		content = m.responses[m.callCount]
	}
	m.callCount++

	// Simulate tool call if configured
	var toolCalls []tools.ToolCall
	if m.simulateTools && len(req.Tools) > 0 {
		toolCalls = []tools.ToolCall{
			{
				ID:   "call_mock_123",
				Type: "function",
				Function: tools.FunctionCall{
					Name:      req.Tools[0].Name,
					Arguments: m.toolCallResult,
				},
			},
		}
	}

	// Calculate mock token counts
	inputTokens := 0
	for _, msg := range req.Messages {
		inputTokens += len(msg.Content) / 4
	}
	outputTokens := len(content) / 4

	// Add thinking content if simulated
	reasoning := ""
	if m.simulateThinking {
		reasoning = m.thinkingContent
	}

	return &llm.Response{
		Content:          content,
		ReasoningContent: reasoning,
		ToolCalls:        toolCalls,
		StopReason:       "end_turn",
		Usage: llm.Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  inputTokens + outputTokens,
		},
		Model: "mock-model-1.0",
	}, nil
}

// GetCallCount returns the number of calls made to the mock client
func (m *MockClient) GetCallCount() int {
	return m.callCount
}
