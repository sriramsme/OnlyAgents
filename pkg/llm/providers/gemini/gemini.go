// Package gemini provides a Gemini LLM client for OnlyAgents
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

const (
	// defaultModel       = "gemini-2.5-flash"
	defaultTemperature = 0.7
	defaultMaxTokens   = 8192
)

func init() {
	llm.RegisterProvider(llm.ProviderGemini, llm.ProviderRegistration{
		Models: []string{
			"gemini-2.5-flash",
			"gemini-2.5-pro",
			"gemini-2.0-flash",
			"gemini-2.0-flash-lite",
		},
		EnvKey: "GEMINI_API_KEY",
		Constructor: func(cfg llm.ProviderConfig) (llm.Client, error) {
			return NewGeminiClient(cfg)
		},
	})
}

// GeminiClient implements llm.Client for Google Gemini
type GeminiClient struct {
	client      *genai.Client
	model       string
	temperature float64
	maxTokens   int
}

// NewGeminiClient creates a new Gemini client
func NewGeminiClient(cfg llm.ProviderConfig) (*GeminiClient, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("gemini: model is required. Supported models are %s", strings.Join(llm.SupportedModels(llm.ProviderGemini), ", "))
	}

	apiKey, err := llm.GetAPIKeyFromVault(cfg.Vault, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to create client: %w", err)
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	temperature := cfg.Temperature
	if temperature == 0 {
		temperature = defaultTemperature
	}

	return &GeminiClient{
		client:      client,
		model:       cfg.Model,
		maxTokens:   maxTokens,
		temperature: temperature,
	}, nil
}

// Chat sends a request to Gemini and returns a response
func (c *GeminiClient) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	start := time.Now()

	contents, systemInstruction, err := toGeminiContents(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to convert messages: %w", err)
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.maxTokens
	}

	temperature := req.Temperature
	if temperature == 0 {
		temperature = c.temperature
	}

	if maxTokens <= 0 || maxTokens > math.MaxInt32 {
		return nil, fmt.Errorf("invalid maxTokens: %d", maxTokens)
	}
	genCfg := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(maxTokens),
		Temperature:     genai.Ptr(float32(temperature)),
	}

	if systemInstruction != "" {
		genCfg.SystemInstruction = genai.NewContentFromText(systemInstruction, "user")
	}

	if len(req.Tools) > 0 {
		genCfg.Tools = toGeminiTools(req.Tools)
	}

	logger.Log.Debug("gemini request",
		"model", c.model,
		"messages", len(req.Messages),
		"tools", len(req.Tools),
		"max_tokens", maxTokens)

	resp, err := c.client.Models.GenerateContent(ctx, c.model, contents, genCfg)
	if err != nil {
		logger.Log.Error("gemini api error", "model", c.model, "error", err)
		return nil, fmt.Errorf("gemini: generate content failed: %w", err)
	}

	result, err := fromGeminiResponse(resp, c.model)
	if err != nil {
		return nil, err
	}

	logger.Log.Info("gemini response",
		"model", c.model,
		"prompt_tokens", result.Usage.InputTokens,
		"completion_tokens", result.Usage.OutputTokens,
		"total_tokens", result.Usage.TotalTokens,
		"has_tool_calls", result.HasToolCalls(),
		"tool_call_count", len(result.ToolCalls),
		"finish_reason", result.StopReason,
		"latency_ms", time.Since(start).Milliseconds())

	return result, nil
}

// Provider returns the provider name
func (c *GeminiClient) Provider() llm.Provider {
	return llm.ProviderGemini
}

// Model returns the model name
func (c *GeminiClient) Model() string {
	return c.model
}

// toGeminiContents converts llm.Messages to genai.Contents.
// System messages are extracted separately because Gemini handles them via
// GenerateContentConfig.SystemInstruction, not as conversation turns.
func toGeminiContents(messages []llm.Message) ([]*genai.Content, string, error) {
	var contents []*genai.Content
	var systemInstruction string

	for _, msg := range messages {
		switch msg.Role {
		case llm.RoleSystem:
			systemInstruction = msg.Content

		case llm.RoleUser:
			contents = append(contents, genai.NewContentFromText(msg.Content, "user"))

		case llm.RoleAssistant:
			content, err := toGeminiAssistantContent(msg)
			if err != nil {
				return nil, "", err
			}
			contents = append(contents, content)

		case llm.RoleTool:
			// Tool results go back as user-turn FunctionResponse parts
			contents = append(contents, toGeminiFunctionResponseContent(msg))
		}
	}

	return contents, systemInstruction, nil
}

// toGeminiAssistantContent converts an assistant message (possibly with tool calls)
// into a Gemini "model" role content block.
func toGeminiAssistantContent(msg llm.Message) (*genai.Content, error) {
	var parts []*genai.Part

	if msg.Content != "" {
		parts = append(parts, &genai.Part{Text: msg.Content})
	}

	for _, tc := range msg.ToolCalls {
		args, err := parseArgsToMap(tc.Function.Arguments)
		if err != nil {
			return nil, fmt.Errorf("gemini: failed to parse tool call args for %s: %w", tc.Function.Name, err)
		}
		parts = append(parts, &genai.Part{
			FunctionCall: &genai.FunctionCall{
				Name: tc.Function.Name,
				Args: args,
			},
		})
	}

	return genai.NewContentFromParts(parts, "model"), nil
}

// toGeminiFunctionResponseContent wraps a tool result message into the format Gemini expects.
func toGeminiFunctionResponseContent(msg llm.Message) *genai.Content {
	return genai.NewContentFromFunctionResponse(msg.Name, map[string]any{
		"result": msg.Content,
	}, "user")
}

// toGeminiTools converts []llm.ToolDef into a single genai.Tool with FunctionDeclarations.
func toGeminiTools(tools []llm.ToolDef) []*genai.Tool {
	decls := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		decls = append(decls, &genai.FunctionDeclaration{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  toGeminiSchema(t.Function.Parameters),
		})
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// toGeminiSchema converts a JSON Schema map (as used in FunctionDef.Parameters)
// into a genai.Schema for the Gemini API.
func toGeminiSchema(params map[string]any) *genai.Schema {
	if params == nil {
		return nil
	}

	schema := &genai.Schema{}

	if t, ok := params["type"].(string); ok {
		schema.Type = genai.Type(strings.ToUpper(t))
	}
	if desc, ok := params["description"].(string); ok {
		schema.Description = desc
	}
	if props, ok := params["properties"].(map[string]any); ok {
		schema.Properties = make(map[string]*genai.Schema, len(props))
		for k, v := range props {
			if propMap, ok := v.(map[string]any); ok {
				schema.Properties[k] = toGeminiSchema(propMap)
			}
		}
	}
	if required, ok := params["required"].([]any); ok {
		for _, r := range required {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}

	return schema
}

// fromGeminiResponse converts a genai.GenerateContentResponse into an llm.Response.
func fromGeminiResponse(resp *genai.GenerateContentResponse, model string) (*llm.Response, error) {
	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: no candidates in response")
	}

	candidate := resp.Candidates[0]
	var textBuilder strings.Builder
	var toolCalls []llm.ToolCall

	if candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				textBuilder.WriteString(part.Text)
			}
			if part.FunctionCall != nil {
				tc, err := fromGeminiFunctionCall(part.FunctionCall)
				if err != nil {
					return nil, err
				}
				toolCalls = append(toolCalls, tc)
			}
		}
	}

	return &llm.Response{
		Content:    textBuilder.String(),
		ToolCalls:  toolCalls,
		StopReason: string(candidate.FinishReason),
		Usage:      extractGeminiUsage(resp.UsageMetadata),
		Model:      model,
	}, nil
}

// fromGeminiFunctionCall converts a genai.FunctionCall to an llm.ToolCall.
// Note: Gemini doesn't generate unique call IDs like OpenAI does (e.g. "call_abc123").
// We use the function name as the ID. If your agent loop calls the same tool multiple
// times in one turn, consider generating a UUID here instead.
func fromGeminiFunctionCall(fc *genai.FunctionCall) (llm.ToolCall, error) {
	argsJSON, err := json.Marshal(fc.Args)
	if err != nil {
		return llm.ToolCall{}, fmt.Errorf("gemini: failed to marshal function call args: %w", err)
	}

	return llm.ToolCall{
		ID:   fc.Name,
		Type: "function",
		Function: llm.FunctionCall{
			Name:      fc.Name,
			Arguments: string(argsJSON),
		},
	}, nil
}

// extractGeminiUsage pulls token counts out of the response metadata.
func extractGeminiUsage(meta *genai.GenerateContentResponseUsageMetadata) llm.Usage {
	if meta == nil {
		return llm.Usage{}
	}
	return llm.Usage{
		InputTokens:  int(meta.PromptTokenCount),
		OutputTokens: int(meta.CandidatesTokenCount),
		TotalTokens:  int(meta.TotalTokenCount),
	}
}

// parseArgsToMap deserializes a JSON string of tool call arguments into a map.
func parseArgsToMap(arguments string) (map[string]any, error) {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return nil, err
	}
	return result, nil
}
