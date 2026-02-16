package llm

import (
    "context"
)

// MockClient is a mock LLM client for testing
type MockClient struct {
    responses []string
    callCount int
}

// NewMockClient creates a mock client with predefined responses
func NewMockClient(responses ...string) *MockClient {
    return &MockClient{
        responses: responses,
    }
}

func (m *MockClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
    response := "Mock response"
    if m.callCount < len(m.responses) {
        response = m.responses[m.callCount]
    }
    m.callCount++

    return &CompletionResponse{
        Content:      response,
        StopReason:   "end_turn",
        InputTokens:  100,
        OutputTokens: 50,
        Model:        "mock-model",
    }, nil
}

func (m *MockClient) Provider() Provider {
    return "mock"
}

func (m *MockClient) Model() string {
    return "mock-model"
}
