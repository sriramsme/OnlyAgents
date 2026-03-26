package openai

import (
	"encoding/base64"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/media"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func mapStopReason(r string) llm.StopReason {
	switch r {
	case "stop":
		return llm.StopReasonEnd
	case "length":
		return llm.StopReasonLength
	case "tool_calls":
		return llm.StopReasonTool
	case "content_filter":
		return llm.StopReasonContent
	default:
		return llm.StopReasonUnknown
	}
}

// buildChatParams creates the parameters for both streaming and non-streaming
func (c *OpenAIClient) buildChatParams(req *llm.Request) openai.ChatCompletionNewParams {
	messages := c.toOpenAIMessages(req.Messages)
	toolParams := toOpenAITools(req.Tools)

	params := openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: messages,
	}

	caps := c.Capabilities()
	// Max tokens (capped by model limits)
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.MaxTokens()
	}
	if maxTokens > caps.MaxTokens {
		maxTokens = caps.MaxTokens
	}
	params.MaxCompletionTokens = openai.Int(int64(maxTokens))

	// Temperature (constrained by model)
	if caps.SupportsTemperature {
		temp := req.Temperature
		if temp == 0 {
			temp = c.Temperature()
		}
		// Clamp to model's valid range
		if temp < caps.MinTemperature {
			temp = caps.MinTemperature
		}
		if temp > caps.MaxTemperature {
			temp = caps.MaxTemperature
		}
		params.Temperature = openai.Float(temp)
	}

	// Tools (only if supported)
	if len(toolParams) > 0 && caps.SupportsToolCalling {
		params.Tools = toolParams
	}

	// Prompt caching (only if supported)
	if c.CachingEnabled() && caps.SupportsPromptCaching {
		params.PromptCacheKey = openai.String(c.CacheKey())
		params.PromptCacheRetention = openai.ChatCompletionNewParamsPromptCacheRetention("24h")
	}
	return params
}

// toOpenAITools converts tools to OpenAI format
func toOpenAITools(tools []tools.ToolDef) []openai.ChatCompletionToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		tool := openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  openai.FunctionParameters(t.Parameters),
		})
		result = append(result, tool)
	}

	return result
}

// parseResponse extracts content and tool calls
func parseResponse(completion *openai.ChatCompletion) *llm.Response {
	if len(completion.Choices) == 0 {
		return &llm.Response{
			Content:   "",
			ToolCalls: []tools.ToolCall{},
			Usage: llm.Usage{
				InputTokens:  int(completion.Usage.PromptTokens),
				OutputTokens: int(completion.Usage.CompletionTokens),
				TotalTokens:  int(completion.Usage.TotalTokens),
				// CachedTokens: int(completion.Usage.PromptTokensCached),
			},
			Model: string(completion.Model),
		}
	}

	choice := completion.Choices[0]
	message := choice.Message

	var toolCalls []tools.ToolCall
	if len(message.ToolCalls) > 0 {
		toolCalls = make([]tools.ToolCall, 0, len(message.ToolCalls))
		for _, tc := range message.ToolCalls {
			toolCalls = append(toolCalls, tools.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: tools.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	return &llm.Response{
		Content:    message.Content,
		ToolCalls:  toolCalls,
		StopReason: mapStopReason(choice.FinishReason),
		Usage: llm.Usage{
			InputTokens:  int(completion.Usage.PromptTokens),
			OutputTokens: int(completion.Usage.CompletionTokens),
			TotalTokens:  int(completion.Usage.TotalTokens),
			// CachedTokens: int(completion.Usage.PromptTokensCached),
		},
		Model: string(completion.Model),
	}
}

// toOpenAIMessages converts the provider-agnostic message slice into the
// OpenAI wire format. Multimodal messages (msg.IsMultimodal() == true) are
// expanded into typed content parts; plain text messages continue to use the
// simple string helpers, identical to the existing behaviour.
func (c *OpenAIClient) toOpenAIMessages(messages []llm.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case llm.RoleSystem:
			result = append(result, openai.SystemMessage(msg.Content))

		case llm.RoleUser:
			if msg.IsMultimodal() {
				result = append(result, c.toOpenAIUserMultimodal(msg))
			} else {
				result = append(result, openai.UserMessage(msg.Content))
			}

		case llm.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					toolCall := openai.ChatCompletionMessageFunctionToolCallParam{
						ID:   tc.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &toolCall})
				}
				assistant := openai.ChatCompletionAssistantMessageParam{}
				assistant.Content.OfString = openai.String(msg.Content)
				assistant.ToolCalls = toolCalls
				result = append(result, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
			} else {
				result = append(result, openai.AssistantMessage(msg.Content))
			}

		case llm.RoleTool:
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		}
	}

	return result
}

// toOpenAIUserMultimodal converts a multimodal user message into an OpenAI
// user message with a typed content part array.
func (c *OpenAIClient) toOpenAIUserMultimodal(msg llm.Message) openai.ChatCompletionMessageParamUnion {
	parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(msg.Parts))
	caps := c.Capabilities()

	for _, part := range msg.Parts {
		switch part.Type {
		case llm.ContentPartTypeText:
			parts = append(parts, openai.TextContentPart(part.Text))

		case llm.ContentPartTypeImage:
			if !caps.SupportsVision {
				parts = append(parts, openai.TextContentPart(
					fmt.Sprintf("[Image attached: %s — this model does not support vision]", part.Filename),
				))
				continue
			}
			parts = append(parts, openai.ImageContentPart(
				openai.ChatCompletionContentPartImageImageURLParam{
					URL: toDataURL(part.MIMEType, part.Data),
				},
			))

		case llm.ContentPartTypeDocument:
			if part.MIMEType == "application/pdf" {
				// PDFs sent natively — model handles layout, tables, diagrams.
				// All vision-capable models from OpenAI, Anthropic, and Gemini
				// support this. Costs vision tokens per page.
				filePart := openai.ChatCompletionContentPartFileParam{
					File: openai.ChatCompletionContentPartFileFileParam{
						FileData: openai.String(toDataURL(part.MIMEType, part.Data)),
						Filename: openai.String(part.Filename),
					},
				}
				parts = append(parts, openai.ChatCompletionContentPartUnionParam{
					OfFile: &filePart,
				})
			} else if text, ok := media.ExtractText(part.Data); ok {
				// All text-based formats: source code, JSON, YAML, CSV,
				// Markdown, config files etc. — inline as text.
				parts = append(parts, openai.TextContentPart(text))
			} else {
				// Truly binary and not an image or PDF.
				parts = append(parts, openai.TextContentPart(
					fmt.Sprintf("[Attached file: %s (%s) — binary format not supported]",
						part.Filename, part.MIMEType),
				))
			}
		}
	}

	user := openai.ChatCompletionUserMessageParam{
		Content: openai.ChatCompletionUserMessageParamContentUnion{
			OfArrayOfContentParts: parts,
		},
	}
	return openai.ChatCompletionMessageParamUnion{OfUser: &user}
}

// toDataURL encodes raw bytes as a data URI.
// Format: "data:<mimeType>;base64,<base64data>"
func toDataURL(mimeType string, data []byte) string {
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
}
