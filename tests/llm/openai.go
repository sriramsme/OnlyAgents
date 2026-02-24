package llm

import (
	"context"
	"os"
	"testing"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/llm/providers/openai"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func TestOpenAIClientBasic(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client, err := openai.NewOpenAIClient(llm.ProviderConfig{
		Model:       "gpt-3.5-turbo",
		MaxTokens:   500,
		Temperature: 0.7,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	resp, err := client.Chat(ctx, &llm.Request{
		Messages: []llm.Message{
			llm.SystemMessage("You are a helpful assistant."),
			llm.UserMessage("Say hello in 5 words"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Content == "" {
		t.Error("Expected non-empty response")
	}

	t.Logf("Response: %s", resp.Content)
	t.Logf("Tokens: %d input, %d output", resp.Usage.InputTokens, resp.Usage.OutputTokens)
}

func TestOpenAIToolCalling(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client, err := openai.NewOpenAIClient(llm.ProviderConfig{
		Model: "gpt-4-turbo",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Define a weather tool
	weatherTool := tools.NewToolDef(
		"get_weather",
		"Get the current weather for a location",
		tools.BuildParams(
			map[string]tools.Property{
				"location": tools.StringProp("The city and state, e.g. San Francisco, CA"),
				"unit": tools.EnumProp(
					"Temperature unit",
					[]string{"celsius", "fahrenheit"},
				),
			},
			[]string{"location"},
		),
	)

	ctx := context.Background()

	resp, err := client.Chat(ctx, &llm.Request{
		Messages: []llm.Message{
			llm.UserMessage("What's the weather in New York?"),
		},
		Tools: []tools.ToolDef{weatherTool}, // <-- ED
	})
	if err != nil {
		t.Fatal(err)
	}

	if !resp.HasToolCalls() {
		t.Error("Expected tool call")
		return
	}

	t.Logf("Tool called: %s", resp.ToolCalls[0].Function.Name)
	t.Logf("Arguments: %s", resp.ToolCalls[0].Function.Arguments)

	// Parse arguments
	args, err := llm.ParseToolArguments(resp.ToolCalls[0].Function.Arguments)
	if err != nil {
		t.Fatal(err)
	}

	location, ok := args["location"].(string)
	if !ok || location == "" {
		t.Error("Expected location in arguments")
	}

	t.Logf("Parsed location: %s", location)
}

func TestOpenAIWithFactory(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	// Mock config
	cfg := &struct {
		LLM struct {
			Provider string
			Model    string
			APIKey   string
			Options  map[string]string
		}
	}{}
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-3.5-turbo"
	cfg.LLM.APIKey = apiKey
	cfg.LLM.Options = map[string]string{
		"max_tokens":  "500",
		"temperature": "0.7",
	}

	// Note: This test would work with actual factory implementation
	// For now, create client directly
	client, err := openai.NewOpenAIClient(llm.ProviderConfig{
		Model:       cfg.LLM.Model,
		MaxTokens:   500,
		Temperature: 0.7,
	})
	if err != nil {
		t.Fatal(err)
	}

	if client.Provider() != llm.ProviderOpenAI {
		t.Errorf("Expected provider openai, got %s", client.Provider())
	}

	if client.Model() != "gpt-3.5-turbo" {
		t.Errorf("Expected model gpt-3.5-turbo, got %s", client.Model())
	}
}

func TestOpenAIMultiTurn(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client, err := openai.NewOpenAIClient(llm.ProviderConfig{
		Model: "gpt-3.5-turbo",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Build conversation
	messages := []llm.Message{
		llm.SystemMessage("You are a helpful assistant."),
		llm.UserMessage("My name is Alice."),
	}

	// Turn 1
	resp1, err := client.Chat(ctx, &llm.Request{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Turn 1: %s", resp1.Content)

	// Add response to history
	messages = append(messages, llm.AssistantMessage(resp1.Content))

	// Turn 2
	messages = append(messages, llm.UserMessage("What's my name?"))
	resp2, err := client.Chat(ctx, &llm.Request{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Turn 2: %s", resp2.Content)

	// Should mention Alice in the response
	if !contains(resp2.Content, "Alice") {
		t.Log("Warning: Expected response to mention 'Alice'")
	}
}

func TestOpenAIStreamingClient(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client, err := openai.NewOpenAIStreamingClient(llm.ProviderConfig{
		Model: "gpt-3.5-turbo",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	resp, err := client.Chat(ctx, &llm.Request{
		Messages: []llm.Message{
			llm.UserMessage("Count from 1 to 5"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Content == "" {
		t.Error("Expected non-empty response")
	}

	t.Logf("Streaming response: %s", resp.Content)
	t.Logf("Tokens: %d", resp.Usage.TotalTokens)
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) >= len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				containsInMiddle(s, substr)))
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
