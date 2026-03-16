package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

const (
	anthropicAPIBase           = "https://api.anthropic.com"
	anthropicThinkingMinBudget = 1024
	anthropicDefaultBudget     = 2048
	anthropicMaxRetries        = 2
)

func init() {
	modelNames := llm.GetSupportedModels(ModelRegistry)

	llm.RegisterProvider(llm.ProviderAnthropic, llm.ProviderRegistration{
		Models:      modelNames,
		EnvKey:      "ANTHROPIC_API_KEY",
		Constructor: NewAnthropicClient,
	})
}

// AnthropicClient implements llm.Client for Anthropic's Claude
type AnthropicClient struct {
	client       *anthropic.Client
	model        string
	capabilities llm.ModelCapabilities

	// Resolved configuration
	maxTokens     int
	temperature   float64
	enableCaching bool
	cacheKey      string
}

// NewAnthropicClient creates a new Anthropic client
func NewAnthropicClient(cfg llm.ProviderConfig) (llm.Client, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("anthropic: model is required")
	}

	// Get model capabilities from registry
	caps, err := llm.GetModelCapabilities(cfg.Model, ModelRegistry)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithMaxRetries(anthropicMaxRetries),
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = anthropicAPIBase
	}
	opts = append(opts, option.WithBaseURL(baseURL))

	maxTokens := caps.DefaultMaxTokens
	temperature := caps.DefaultTemperature

	if cfg.Options != nil {
		if cfg.Options.MaxTokens > 0 {
			maxTokens = cfg.Options.MaxTokens
		}

		if cfg.Options.Temperature > 0 {
			temperature = cfg.Options.Temperature
		}
	}

	// Validate the final configuration
	if maxTokens > caps.MaxTokens {
		return nil, fmt.Errorf("max_tokens %d exceeds model limit %d", maxTokens, caps.MaxTokens)
	}

	if caps.SupportsTemperature {
		if temperature < caps.MinTemperature || temperature > caps.MaxTemperature {
			return nil, fmt.Errorf("temperature %.2f outside valid range [%.2f, %.2f]",
				temperature, caps.MinTemperature, caps.MaxTemperature)
		}
	}

	client := anthropic.NewClient(opts...)

	return &AnthropicClient{
		client:        &client,
		model:         cfg.Model,
		capabilities:  caps,
		maxTokens:     maxTokens,
		temperature:   temperature,
		enableCaching: caps.SupportsPromptCaching,
		cacheKey:      fmt.Sprintf("agent-%s-%d", cfg.Model, time.Now().Unix()/3600),
	}, nil
}

// Chat sends a non-streaming chat completion request
func (c *AnthropicClient) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	start := time.Now()

	// Runtime validation
	if len(req.Tools) > 0 && !c.capabilities.SupportsToolCalling {
		return nil, fmt.Errorf("model %s does not support tool calling", c.model)
	}

	params, err := c.buildMessageParams(req)
	if err != nil {
		return nil, err
	}

	logger.Log.Debug("anthropic request",
		"model", c.model,
		"messages", len(req.Messages),
		"tools", len(req.Tools),
		"streaming", false)

	message, err := c.client.Messages.New(ctx, *params)
	if err != nil {
		logger.Log.Error("anthropic api error", "model", c.model, "error", err)
		return nil, fmt.Errorf("anthropic api error: %w", err)
	}

	resp := c.parseResponse(message)

	logger.Log.Info("anthropic response",
		"model", c.model,
		"input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens,
		"has_tool_calls", resp.HasToolCalls(),
		"has_reasoning", resp.ReasoningContent != "",
		"latency_ms", time.Since(start).Milliseconds())

	return resp, nil
}

// ChatStream sends a streaming chat completion request
func (c *AnthropicClient) ChatStream(ctx context.Context, req *llm.Request) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)

	go func() {
		defer close(ch)

		// Validate capabilities
		if err := c.validateStreamingCapabilities(req); err != nil {
			ch <- llm.StreamChunk{Error: err, Done: true}
			return
		}

		start := time.Now()

		params, err := c.buildMessageParams(req)
		if err != nil {
			ch <- llm.StreamChunk{Error: fmt.Errorf("anthropic: %w", err), Done: true}
			return
		}

		logger.Log.Debug("anthropic streaming request",
			"model", c.model,
			"messages", len(req.Messages),
			"tools", len(req.Tools),
			"streaming", true)

		// Create streaming request
		stream := c.client.Messages.NewStreaming(ctx, *params)

		// Process stream
		message := anthropic.Message{}
		var currentToolCall *tools.ToolCall
		var currentToolJSON strings.Builder

		for stream.Next() {
			event := stream.Current()

			// Accumulate into message
			if err := message.Accumulate(event); err != nil {
				logger.Log.Error("failed to accumulate message", "error", err)
				ch <- llm.StreamChunk{Error: err, Done: true}
				return
			}

			// Handle event
			c.handleStreamEvent(event, ch, &currentToolCall, &currentToolJSON)
		}

		if err := stream.Err(); err != nil {
			logger.Log.Error("anthropic stream error", "error", err)
			ch <- llm.StreamChunk{Error: err, Done: true}
			return
		}

		// Send completion signal
		ch <- llm.StreamChunk{Done: true}

		logger.Log.Info("anthropic streaming complete",
			"model", c.model,
			"input_tokens", message.Usage.InputTokens,
			"output_tokens", message.Usage.OutputTokens,
			"latency_ms", time.Since(start).Milliseconds())
	}()

	return ch
}

// validateStreamingCapabilities checks if the model supports required features
func (c *AnthropicClient) validateStreamingCapabilities(req *llm.Request) error {
	if !c.capabilities.SupportsStreaming {
		return fmt.Errorf("model %s does not support streaming", c.model)
	}
	if len(req.Tools) > 0 && !c.capabilities.SupportsToolCalling {
		return fmt.Errorf("model %s does not support tool calling", c.model)
	}
	return nil
}

// handleStreamEvent processes individual stream events
func (c *AnthropicClient) handleStreamEvent(event anthropic.MessageStreamEventUnion, ch chan<- llm.StreamChunk, currentToolCall **tools.ToolCall, currentToolJSON *strings.Builder) {
	switch ev := event.AsAny().(type) {
	case anthropic.MessageStartEvent:
		// Message started, nothing to emit yet

	case anthropic.ContentBlockStartEvent:
		c.handleContentBlockStart(ev, currentToolCall, currentToolJSON)

	case anthropic.ContentBlockDeltaEvent:
		c.handleContentBlockDelta(ev, ch, *currentToolCall, currentToolJSON)

	case anthropic.ContentBlockStopEvent:
		c.handleContentBlockStop(ch, currentToolCall, currentToolJSON)

	case anthropic.MessageDeltaEvent:
		// Message delta (usage updates)

	case anthropic.MessageStopEvent:
		// Message complete
	}
}

// handleContentBlockStart handles the start of a new content block
func (c *AnthropicClient) handleContentBlockStart(ev anthropic.ContentBlockStartEvent, currentToolCall **tools.ToolCall, currentToolJSON *strings.Builder) {
	if ev.ContentBlock.Type == "tool_use" {
		*currentToolCall = &tools.ToolCall{
			ID:   ev.ContentBlock.ID,
			Type: "function",
			Function: tools.FunctionCall{
				Name: ev.ContentBlock.Name,
			},
		}
		currentToolJSON.Reset()
	}
}

// handleContentBlockDelta handles delta events within a content block
func (c *AnthropicClient) handleContentBlockDelta(ev anthropic.ContentBlockDeltaEvent, ch chan<- llm.StreamChunk, currentToolCall *tools.ToolCall, currentToolJSON *strings.Builder) {
	switch delta := ev.Delta.AsAny().(type) {
	case anthropic.TextDelta:
		ch <- llm.StreamChunk{Content: delta.Text}

	case anthropic.InputJSONDelta:
		if currentToolCall != nil {
			currentToolJSON.WriteString(delta.PartialJSON)
		}
	}
}

// handleContentBlockStop handles the completion of a content block
func (c *AnthropicClient) handleContentBlockStop(ch chan<- llm.StreamChunk, currentToolCall **tools.ToolCall, currentToolJSON *strings.Builder) {
	if *currentToolCall != nil {
		(*currentToolCall).Function.Arguments = currentToolJSON.String()
		ch <- llm.StreamChunk{ToolCalls: []tools.ToolCall{**currentToolCall}}
		*currentToolCall = nil
		currentToolJSON.Reset()
	}
}

// buildMessageParams creates the parameters for both streaming and non-streaming
func (c *AnthropicClient) buildMessageParams(req *llm.Request) (*anthropic.MessageNewParams, error) {
	// Convert messages to Anthropic format
	systemPrompt, messages, err := c.toAnthropicMessages(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	maxTokens := c.calculateMaxTokens(req.MaxTokens)
	thinkingEnabled := c.isThinkingModel()

	// Build base params
	params := &anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		Messages:  messages,
		MaxTokens: int64(maxTokens),
	}

	// Add system prompt
	c.addSystemPrompt(params, systemPrompt)

	// Add tools
	c.addTools(params, req.Tools)

	// Set temperature
	c.setTemperature(params, req.Temperature, thinkingEnabled)

	// Enable thinking if applicable
	if thinkingEnabled {
		if budget, ok := c.calculateThinkingBudget(maxTokens); ok {
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
			logger.Log.Debug("enabled extended thinking", "budget", budget)
		}
	}

	return params, nil
}

// calculateMaxTokens determines and validates the max tokens value
func (c *AnthropicClient) calculateMaxTokens(requestedTokens int) int {
	maxTokens := requestedTokens
	if maxTokens == 0 {
		maxTokens = c.maxTokens
	}
	if maxTokens > c.capabilities.MaxTokens {
		maxTokens = c.capabilities.MaxTokens
	}

	thinkingEnabled := c.isThinkingModel()
	if thinkingEnabled && maxTokens <= anthropicThinkingMinBudget {
		logger.Log.Warn("adjusting max_tokens for thinking model",
			"model", c.model,
			"original", maxTokens,
			"adjusted", anthropicDefaultBudget)
		maxTokens = anthropicDefaultBudget
	}

	return maxTokens
}

// addSystemPrompt adds the system prompt to params with optional caching
func (c *AnthropicClient) addSystemPrompt(params *anthropic.MessageNewParams, systemPrompt string) {
	if systemPrompt == "" {
		return
	}

	if c.enableCaching && c.capabilities.SupportsPromptCaching {
		params.System = []anthropic.TextBlockParam{
			{
				Text: systemPrompt,
				CacheControl: anthropic.CacheControlEphemeralParam{
					Type: "ephemeral",
				},
			},
		}
	} else {
		params.System = []anthropic.TextBlockParam{{Text: systemPrompt}}
	}
}

// addTools adds tool definitions to params with optional caching
func (c *AnthropicClient) addTools(params *anthropic.MessageNewParams, requestTools []tools.ToolDef) {
	if len(requestTools) == 0 || !c.capabilities.SupportsToolCalling {
		return
	}

	tools := c.toAnthropicTools(requestTools)
	params.Tools = tools

	// Add cache control to tools if caching is enabled
	if c.enableCaching && c.capabilities.SupportsPromptCaching && len(tools) > 0 {
		if lastTool := tools[len(tools)-1].OfTool; lastTool != nil {
			lastTool.CacheControl = anthropic.CacheControlEphemeralParam{
				Type: "ephemeral",
			}
		}
	}
}

// setTemperature sets the temperature parameter with validation
func (c *AnthropicClient) setTemperature(params *anthropic.MessageNewParams, requestedTemp float64, thinkingEnabled bool) {
	temperature := requestedTemp
	if temperature == 0 {
		temperature = c.temperature
	}

	if thinkingEnabled {
		if temperature != 1.0 {
			logger.Log.Debug("overriding temperature for thinking model",
				"requested", temperature,
				"required", 1.0)
		}
		temperature = 1.0
	} else if c.capabilities.SupportsTemperature {
		// Clamp temperature to valid range
		if temperature < c.capabilities.MinTemperature {
			temperature = c.capabilities.MinTemperature
		}
		if temperature > c.capabilities.MaxTemperature {
			temperature = c.capabilities.MaxTemperature
		}
	}

	params.Temperature = anthropic.Float(temperature)
}

// toAnthropicMessages converts our message format to Anthropic's format
func (c *AnthropicClient) toAnthropicMessages(messages []llm.Message) (string, []anthropic.MessageParam, error) {
	var systemPrompt string
	msgList := make([]anthropic.MessageParam, 0, len(messages))

	// Anthropic requires tool results to be in user messages
	pendingToolResults := make([]anthropic.ContentBlockParamUnion, 0)

	flushPendingToolResults := func() {
		if len(pendingToolResults) == 0 {
			return
		}
		msgList = append(msgList, anthropic.NewUserMessage(pendingToolResults...))
		pendingToolResults = nil
	}

	for i, m := range messages {
		switch m.Role {
		case llm.RoleSystem:
			systemPrompt = m.Content

		case llm.RoleUser:
			flushPendingToolResults()

			// Add cache control to last user message if caching is enabled
			if c.enableCaching && c.capabilities.SupportsPromptCaching && i == len(messages)-1 {
				msgList = append(msgList, anthropic.NewUserMessage(
					anthropic.ContentBlockParamUnion{
						OfText: &anthropic.TextBlockParam{
							Text: m.Content,
							CacheControl: anthropic.CacheControlEphemeralParam{
								Type: "ephemeral",
							},
						},
					},
				))
			} else {
				msgList = append(msgList, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
			}

		case llm.RoleAssistant:
			flushPendingToolResults()

			blocks := make([]anthropic.ContentBlockParamUnion, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}

			// Add tool calls
			for _, tc := range m.ToolCalls {
				input := c.parseToolArguments(tc.Function.Arguments)
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					},
				})
			}

			if len(blocks) > 0 {
				msgList = append(msgList, anthropic.NewAssistantMessage(blocks...))
			}

		case llm.RoleTool:
			// Queue tool results to be added to next user message
			pendingToolResults = append(pendingToolResults, anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: m.ToolCallID,
					Content: []anthropic.ToolResultBlockParamContentUnion{{
						OfText: &anthropic.TextBlockParam{Text: m.Content},
					}},
				},
			})

		default:
			return "", nil, fmt.Errorf("unsupported message role: %s", m.Role)
		}
	}

	flushPendingToolResults()
	return systemPrompt, msgList, nil
}

// toAnthropicTools converts our tool format to Anthropic's format
func (c *AnthropicClient) toAnthropicTools(tools []tools.ToolDef) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	result := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		schema := anthropic.ToolInputSchemaParam{
			ExtraFields: map[string]any{},
		}

		if params := t.Parameters; params != nil {
			if properties, ok := params["properties"]; ok {
				schema.Properties = properties
			}
			if required, ok := params["required"]; ok {
				schema.Required = c.normalizeRequired(required)
			}
			// Copy other fields to ExtraFields
			for k, v := range params {
				if k != "type" && k != "properties" && k != "required" {
					schema.ExtraFields[k] = v
				}
			}
		}

		tool := anthropic.ToolParam{
			Name:        t.Name,
			InputSchema: schema,
		}
		if t.Description != "" {
			tool.Description = anthropic.String(t.Description)
		}

		result = append(result, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return result
}

// parseResponse extracts content, reasoning, and tool calls from the response
func (c *AnthropicClient) parseResponse(message *anthropic.Message) *llm.Response {
	var textParts []string
	var reasoningParts []string
	var toolCalls []tools.ToolCall

	for _, block := range message.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "thinking":
			if strings.TrimSpace(block.Thinking) != "" {
				reasoningParts = append(reasoningParts, strings.TrimSpace(block.Thinking))
			}
		case "redacted_thinking":
			reasoningParts = append(reasoningParts, "[redacted_thinking]")
		case "tool_use":
			toolCalls = append(toolCalls, tools.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: tools.FunctionCall{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		}
	}

	content := strings.Join(textParts, "\n")
	reasoningContent := strings.Join(reasoningParts, "\n")

	return &llm.Response{
		Content:          content,
		ReasoningContent: reasoningContent,
		ToolCalls:        toolCalls,
		StopReason:       string(message.StopReason),
		Usage: llm.Usage{
			InputTokens:  int(message.Usage.InputTokens),
			OutputTokens: int(message.Usage.OutputTokens),
			TotalTokens:  int(message.Usage.InputTokens + message.Usage.OutputTokens),
			CachedTokens: int(message.Usage.CacheReadInputTokens),
		},
		Model: string(message.Model),
	}
}

// isThinkingModel checks if the current model supports extended thinking
func (c *AnthropicClient) isThinkingModel() bool {
	model := strings.ToLower(c.model)
	return strings.Contains(model, "sonnet-4-5") ||
		strings.Contains(model, "opus-4-6") ||
		strings.Contains(model, "claude-sonnet-4-5") ||
		strings.Contains(model, "claude-opus-4-6")
}

// calculateThinkingBudget calculates the thinking token budget
func (c *AnthropicClient) calculateThinkingBudget(maxTokens int) (int64, bool) {
	if maxTokens <= anthropicThinkingMinBudget {
		return 0, false
	}

	budget := anthropicDefaultBudget
	if budget >= maxTokens {
		budget = maxTokens - 1
	}
	if budget < anthropicThinkingMinBudget {
		return 0, false
	}
	return int64(budget), true
}

// parseToolArguments safely parses tool arguments JSON
func (c *AnthropicClient) parseToolArguments(arguments string) any {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return map[string]any{}
	}

	var parsed any
	if err := json.Unmarshal([]byte(arguments), &parsed); err != nil {
		return arguments // Return as string if parsing fails
	}
	return parsed
}

// normalizeRequired normalizes the required field to []string
func (c *AnthropicClient) normalizeRequired(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// Provider returns the provider name
func (c *AnthropicClient) Provider() llm.Provider {
	return llm.ProviderAnthropic
}

// Model returns the model name
func (c *AnthropicClient) Model() string {
	return c.model
}

// SetCaching controls prompt caching
func (c *AnthropicClient) SetCaching(enabled bool) {
	if c.capabilities.SupportsPromptCaching {
		c.enableCaching = enabled
		logger.Log.Debug("anthropic prompt caching", "enabled", enabled)
	}
}

// SetCacheKey sets a custom cache key (for compatibility, not used by Anthropic)
func (c *AnthropicClient) SetCacheKey(key string) {
	c.cacheKey = key
}

// Capabilities returns the model capabilities
func (c *AnthropicClient) Capabilities() llm.ModelCapabilities {
	return c.capabilities
}
