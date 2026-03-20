package anthropic

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/media"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// toAnthropicMessages converts our message format to Anthropic's format.
//
//nolint:gocyclo
func (c *AnthropicClient) toAnthropicMessages(messages []llm.Message) (string, []anthropic.MessageParam, error) {
	var systemPrompt string
	msgList := make([]anthropic.MessageParam, 0, len(messages))

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

			if m.IsMultimodal() {
				blocks, err := c.toAnthropicContentBlocks(m)
				if err != nil {
					return "", nil, fmt.Errorf("build multimodal user message: %w", err)
				}
				// Apply cache control to last user message if caching enabled.
				if c.enableCaching && c.capabilities.SupportsPromptCaching && i == len(messages)-1 {
					if len(blocks) > 0 {
						last := &blocks[len(blocks)-1]
						if last.OfText != nil {
							last.OfText.CacheControl = anthropic.CacheControlEphemeralParam{Type: "ephemeral"}
						}
					}
				}
				msgList = append(msgList, anthropic.NewUserMessage(blocks...))
			} else {
				// Plain text — existing behaviour unchanged.
				if c.enableCaching && c.capabilities.SupportsPromptCaching && i == len(messages)-1 {
					msgList = append(msgList, anthropic.NewUserMessage(
						anthropic.ContentBlockParamUnion{
							OfText: &anthropic.TextBlockParam{
								Text:         m.Content,
								CacheControl: anthropic.CacheControlEphemeralParam{Type: "ephemeral"},
							},
						},
					))
				} else {
					msgList = append(msgList, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
				}
			}

		case llm.RoleAssistant:
			flushPendingToolResults()
			blocks := make([]anthropic.ContentBlockParamUnion, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			for _, tc := range m.ToolCalls {
				input := parseToolArguments(tc.Function.Arguments)
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

// toAnthropicContentBlocks converts a multimodal message's Parts into
// Anthropic content blocks.
func (c *AnthropicClient) toAnthropicContentBlocks(msg llm.Message) ([]anthropic.ContentBlockParamUnion, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Parts))

	for _, part := range msg.Parts {
		switch part.Type {
		case llm.ContentPartTypeText:
			blocks = append(blocks, anthropic.NewTextBlock(part.Text))

		case llm.ContentPartTypeImage:
			if !c.capabilities.SupportsVision {
				blocks = append(blocks, anthropic.NewTextBlock(
					fmt.Sprintf("[Image attached: %s — this model does not support vision]", part.Filename),
				))
				continue
			}
			mediaType, err := toAnthropicImageMediaType(part.MIMEType)
			if err != nil {
				// Unsupported image format — note it rather than failing the turn.
				blocks = append(blocks, anthropic.NewTextBlock(
					fmt.Sprintf("[Image attached: %s — unsupported format %s]", part.Filename, part.MIMEType),
				))
				continue
			}
			blocks = append(blocks, anthropic.ContentBlockParamUnion{
				OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfBase64: &anthropic.Base64ImageSourceParam{
							MediaType: mediaType,
							Data:      base64.StdEncoding.EncodeToString(part.Data),
						},
					},
				},
			})

		case llm.ContentPartTypeDocument:
			if part.MIMEType == "application/pdf" {
				// Anthropic supports PDFs natively as document blocks.
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfDocument: &anthropic.DocumentBlockParam{
						Source: anthropic.DocumentBlockParamSourceUnion{
							OfBase64: &anthropic.Base64PDFSourceParam{
								MediaType: "application/pdf",
								Data:      base64.StdEncoding.EncodeToString(part.Data),
							},
						},
					},
				})
			} else if text, ok := media.ExtractText(part.Data); ok {
				// All text-based formats — Anthropic supports plain text
				// document blocks, but inlining as text is simpler and
				// universally compatible across model versions.
				blocks = append(blocks, anthropic.NewTextBlock(text))
			} else {
				blocks = append(blocks, anthropic.NewTextBlock(
					fmt.Sprintf("[Attached file: %s (%s) — binary format not supported]",
						part.Filename, part.MIMEType),
				))
			}
		}
	}

	return blocks, nil
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
		handleContentBlockStart(ev, currentToolCall, currentToolJSON)

	case anthropic.ContentBlockDeltaEvent:
		handleContentBlockDelta(ev, ch, *currentToolCall, currentToolJSON)

	case anthropic.ContentBlockStopEvent:
		handleContentBlockStop(ch, currentToolCall, currentToolJSON)

	case anthropic.MessageDeltaEvent:
		// Message delta (usage updates)

	case anthropic.MessageStopEvent:
		// Message complete
	}
}

// handleContentBlockStart handles the start of a new content block
func handleContentBlockStart(ev anthropic.ContentBlockStartEvent, currentToolCall **tools.ToolCall, currentToolJSON *strings.Builder) {
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
func handleContentBlockDelta(ev anthropic.ContentBlockDeltaEvent, ch chan<- llm.StreamChunk, currentToolCall *tools.ToolCall, currentToolJSON *strings.Builder) {
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
func handleContentBlockStop(ch chan<- llm.StreamChunk, currentToolCall **tools.ToolCall, currentToolJSON *strings.Builder) {
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
		if budget, ok := calculateThinkingBudget(maxTokens); ok {
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

	tools := toAnthropicTools(requestTools)
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

// toAnthropicTools converts our tool format to Anthropic's format
func toAnthropicTools(tools []tools.ToolDef) []anthropic.ToolUnionParam {
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
				schema.Required = normalizeRequired(required)
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
func parseResponse(message *anthropic.Message) *llm.Response {
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
func calculateThinkingBudget(maxTokens int) (int64, bool) {
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
func parseToolArguments(arguments string) any {
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
func normalizeRequired(value any) []string {
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

// toAnthropicImageMediaType maps a MIME type to Anthropic's typed enum.
// Anthropic only accepts jpeg, png, gif, and webp.
func toAnthropicImageMediaType(mimeType string) (anthropic.Base64ImageSourceMediaType, error) {
	switch mimeType {
	case "image/jpeg":
		return anthropic.Base64ImageSourceMediaTypeImageJPEG, nil
	case "image/png":
		return anthropic.Base64ImageSourceMediaTypeImagePNG, nil
	case "image/gif":
		return anthropic.Base64ImageSourceMediaTypeImageGIF, nil
	case "image/webp":
		return anthropic.Base64ImageSourceMediaTypeImageWebP, nil
	}
	return "", fmt.Errorf("unsupported image media type: %s", mimeType)
}
