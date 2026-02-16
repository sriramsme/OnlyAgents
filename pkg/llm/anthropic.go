package llm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicClient implements Client for Anthropic's Claude
type AnthropicClient struct {
	client      anthropic.Client
	model       string
	maxTokens   int
	temperature float64
}

// NewAnthropicClient creates a new Anthropic client
func NewAnthropicClient(config Config) (*AnthropicClient, error) {
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}

	if config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(config.BaseURL))
	}

	client := anthropic.NewClient(opts...)

	model := config.Model
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	maxTokens := config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	temperature := config.Temperature
	if temperature == 0 {
		temperature = 1.0
	}

	return &AnthropicClient{
		client:      client,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
	}, nil
}

// Complete sends a completion request to Claude
func (c *AnthropicClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	// Convert our messages to Anthropic format
	messages := make([]anthropic.MessageParam, 0, len(req.Messages))
	for _, msg := range req.Messages {
		var m anthropic.MessageParam

		switch msg.Role {
		case RoleAssistant:
			m = anthropic.NewAssistantMessage(
				anthropic.NewTextBlock(msg.Content),
			)
		default: // treat everything else as user
			m = anthropic.NewUserMessage(
				anthropic.NewTextBlock(msg.Content),
			)
		}

		messages = append(messages, m)
	}

	// Set max tokens
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.maxTokens
	}

	// Set temperature
	temperature := req.Temperature
	if temperature == 0 {
		temperature = c.temperature
	}

	// Build request params
	params := anthropic.MessageNewParams{
		Model:       anthropic.Model(c.model),
		Messages:    messages,
		MaxTokens:   int64(maxTokens),
		Temperature: anthropic.Float(temperature),
	}

	// Add system prompt if provided
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{
			{
				Text: req.System,
			},
		}
	}

	// Make the request
	message, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic api error: %w", err)
	}

	// Extract text content
	var builder strings.Builder

	for _, block := range message.Content {
		if block.Type == "text" && block.Text != "" {
			builder.WriteString(block.Text)
		}
	}

	content := builder.String()

	return &CompletionResponse{
		Content:      content,
		StopReason:   string(message.StopReason),
		InputTokens:  int(message.Usage.InputTokens),
		OutputTokens: int(message.Usage.OutputTokens),
		Model:        string(message.Model),
	}, nil
}

// Provider returns the provider name
func (c *AnthropicClient) Provider() Provider {
	return ProviderAnthropic
}

// Model returns the model name
func (c *AnthropicClient) Model() string {
	return c.model
}
