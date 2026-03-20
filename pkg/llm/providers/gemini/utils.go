package gemini

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/media"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// validateStreamingCapabilities checks if the model supports required features
func (c *GeminiClient) validateStreamingCapabilities(req *llm.Request) error {
	if !c.capabilities.SupportsStreaming {
		return fmt.Errorf("model %s does not support streaming", c.model)
	}
	if len(req.Tools) > 0 && !c.capabilities.SupportsToolCalling {
		return fmt.Errorf("model %s does not support tool calling", c.model)
	}
	return nil
}

// streamingMetrics tracks token usage during streaming
type streamingMetrics struct {
	InputTokens  int
	OutputTokens int
	CachedTokens int
}

// processStreamingResults handles the streaming response and accumulates results
func processStreamingResults(ctx context.Context, chat *genai.Chat, parts []genai.Part, ch chan<- llm.StreamChunk) streamingMetrics {
	var metrics streamingMetrics
	toolCallsMap := make(map[string]*tools.ToolCall)

	for result, err := range chat.SendMessageStream(ctx, parts...) {
		if err != nil {
			logger.Log.Error("gemini stream error", "error", err)
			ch <- llm.StreamChunk{Error: err, Done: true}
			return metrics
		}

		if result == nil || len(result.Candidates) == 0 {
			continue
		}

		candidate := result.Candidates[0]
		if candidate.Content == nil {
			continue
		}

		// Process parts in the chunk
		processStreamChunkParts(candidate.Content.Parts, ch, toolCallsMap)

		// Update token counts
		if result.UsageMetadata != nil {
			metrics.InputTokens = int(result.UsageMetadata.PromptTokenCount)
			metrics.OutputTokens = int(result.UsageMetadata.CandidatesTokenCount)
			metrics.CachedTokens = int(result.UsageMetadata.CachedContentTokenCount)
		}
	}

	// Emit any accumulated tool calls
	if len(toolCallsMap) > 0 {
		var completedToolCalls []tools.ToolCall
		for _, tc := range toolCallsMap {
			completedToolCalls = append(completedToolCalls, *tc)
		}
		ch <- llm.StreamChunk{ToolCalls: completedToolCalls}
	}

	return metrics
}

// processStreamChunkParts processes individual parts from a streaming chunk
func processStreamChunkParts(parts []*genai.Part, ch chan<- llm.StreamChunk, toolCallsMap map[string]*tools.ToolCall) {
	for _, part := range parts {
		// Handle text content
		if part.Text != "" {
			ch <- llm.StreamChunk{Content: part.Text}
		}

		// Handle function calls
		if part.FunctionCall != nil {
			tc, err := fromGeminiFunctionCall(part.FunctionCall)
			if err != nil {
				logger.Log.Error("failed to parse function call", "error", err)
				continue
			}
			toolCallsMap[tc.ID] = &tc
		}
	}
}

// buildGenerateConfig creates the configuration for both streaming and non-streaming
func (c *GeminiClient) buildGenerateConfig(req *llm.Request, systemInstruction string, contents []*genai.Content) *genai.GenerateContentConfig {
	maxTokens := c.getValidatedMaxTokens(req.MaxTokens)
	temperature := c.getValidatedTemperature(req.Temperature)

	config := &genai.GenerateContentConfig{
		MaxOutputTokens: maxTokens,
		Temperature:     genai.Ptr(float32(temperature)),
	}

	if systemInstruction != "" {
		config.SystemInstruction = genai.NewContentFromText(systemInstruction, "user")
	}

	// Add tools if provided
	tools := toGeminiTools(req.Tools)
	if len(tools) > 0 && c.capabilities.SupportsToolCalling {
		config.Tools = tools
	}

	// Handle caching if enabled
	c.applyCaching(config, systemInstruction, contents, tools)

	return config
}

// getValidatedMaxTokens validates and returns the max tokens value
func (c *GeminiClient) getValidatedMaxTokens(requestedTokens int) int32 {
	maxTokens := requestedTokens

	if maxTokens == 0 {
		maxTokens = c.maxTokens
	}
	if maxTokens > c.capabilities.MaxTokens {
		maxTokens = c.capabilities.MaxTokens
	}
	if maxTokens <= 0 || maxTokens > math.MaxInt32 {
		maxTokens = c.capabilities.DefaultMaxTokens
	}

	return int32(maxTokens) // #nosec G115 -- value is clamped to int32 bounds above
}

// getValidatedTemperature validates and clamps the temperature value
func (c *GeminiClient) getValidatedTemperature(requestedTemp float64) float64 {
	temperature := requestedTemp
	if temperature == 0 {
		temperature = c.temperature
	}

	if c.capabilities.SupportsTemperature {
		if temperature < c.capabilities.MinTemperature {
			temperature = c.capabilities.MinTemperature
		}
		if temperature > c.capabilities.MaxTemperature {
			temperature = c.capabilities.MaxTemperature
		}
	}

	return temperature
}

// applyCaching applies caching configuration if enabled
func (c *GeminiClient) applyCaching(config *genai.GenerateContentConfig, systemInstruction string, contents []*genai.Content, tools []*genai.Tool) {
	if !c.enableCaching || !c.capabilities.SupportsPromptCaching {
		return
	}

	// Check if we need to create/update cache
	shouldCreateCache := c.cachedContentName == "" || c.contentChanged(contents, tools)

	if shouldCreateCache {
		err := c.createCachedContent(context.Background(), systemInstruction, contents, tools)
		if err != nil {
			logger.Log.Warn("failed to create cached content", "error", err)
		} else {
			logger.Log.Debug("created cached content",
				"name", c.cachedContentName,
				"ttl", c.cacheTTL)
		}
	}

	// Reference the cached content if available
	if c.cachedContentName != "" {
		config.CachedContent = c.cachedContentName
	}
}

// createCachedContent creates a cached content object
func (c *GeminiClient) createCachedContent(ctx context.Context, systemInstruction string, contents []*genai.Content, tools []*genai.Tool) error {
	// Delete old cache if exists
	if c.cachedContentName != "" {
		_, err := c.client.Caches.Delete(ctx, c.cachedContentName, &genai.DeleteCachedContentConfig{})
		if err != nil {
			fmt.Printf("failed to delete cache: %s", err)
		}
	}

	// Prepare contents for caching
	// Gemini caches system instruction + initial contents
	cacheContents := make([]*genai.Content, 0)

	// Add system instruction as first content if present
	if systemInstruction != "" {
		cacheContents = append(cacheContents, genai.NewContentFromText(systemInstruction, "user"))
	}

	// Add initial conversation contents (but not the latest user message)
	if len(contents) > 1 {
		cacheContents = append(cacheContents, contents[:len(contents)-1]...)
	}

	// Only create cache if we have substantial content
	if len(cacheContents) == 0 {
		return nil
	}

	createConfig := &genai.CreateCachedContentConfig{
		TTL:      c.cacheTTL,
		Contents: cacheContents,
	}

	// Add tools to cache if present
	if len(tools) > 0 {
		createConfig.Tools = tools
	}

	result, err := c.client.Caches.Create(ctx, c.model, createConfig)
	if err != nil {
		return fmt.Errorf("failed to create cache: %w", err)
	}

	c.cachedContentName = result.Name
	c.lastCacheContents = cacheContents
	c.lastCacheTools = tools

	return nil
}

// contentChanged checks if content or tools have changed since last cache
func (c *GeminiClient) contentChanged(contents []*genai.Content, tools []*genai.Tool) bool {
	// Simple heuristic: check if lengths changed
	if len(c.lastCacheContents) != len(contents)-1 { // -1 because we don't cache last message
		return true
	}
	if len(c.lastCacheTools) != len(tools) {
		return true
	}
	// In production, you might want to do deep comparison
	return false
}

// DeleteCache deletes the cached content
func (c *GeminiClient) DeleteCache(ctx context.Context) error {
	if c.cachedContentName == "" {
		return nil
	}

	_, err := c.client.Caches.Delete(ctx, c.cachedContentName, &genai.DeleteCachedContentConfig{})
	if err != nil {
		return fmt.Errorf("failed to delete cache: %w", err)
	}

	c.cachedContentName = ""
	c.lastCacheContents = nil
	c.lastCacheTools = nil

	logger.Log.Debug("deleted cached content")
	return nil
}

// GetCacheInfo returns information about the current cache
func (c *GeminiClient) GetCacheInfo(ctx context.Context) (*genai.CachedContent, error) {
	if c.cachedContentName == "" {
		return nil, fmt.Errorf("no cache created")
	}

	result, err := c.client.Caches.Get(ctx, c.cachedContentName, &genai.GetCachedContentConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cache info: %w", err)
	}

	return result, nil
}

// UpdateCacheTTL updates the TTL of the cached content
func (c *GeminiClient) UpdateCacheTTL(ctx context.Context, newTTL time.Duration) error {
	if c.cachedContentName == "" {
		return fmt.Errorf("no cache created")
	}

	c.cacheTTL = newTTL
	expireTime := time.Now().Add(newTTL)

	_, err := c.client.Caches.Update(ctx, c.cachedContentName, &genai.UpdateCachedContentConfig{
		ExpireTime: expireTime,
	})
	if err != nil {
		return fmt.Errorf("failed to update cache TTL: %w", err)
	}

	logger.Log.Debug("updated cache TTL", "ttl", newTTL)
	return nil
}

// toGeminiContents converts llm.Messages to genai.Contents.
func (c *GeminiClient) toGeminiContents(messages []llm.Message) ([]*genai.Content, string, error) {
	var contents []*genai.Content
	var systemInstruction string

	for _, msg := range messages {
		switch msg.Role {
		case llm.RoleSystem:
			systemInstruction = msg.Content

		case llm.RoleUser:
			if msg.IsMultimodal() {
				content, err := c.toGeminiUserMultimodal(msg)
				if err != nil {
					return nil, "", fmt.Errorf("build multimodal user message: %w", err)
				}
				contents = append(contents, content)
			} else {
				contents = append(contents, &genai.Content{
					Role:  "user",
					Parts: []*genai.Part{{Text: msg.Content}},
				})
			}

		case llm.RoleAssistant:
			content, err := toGeminiAssistantContent(msg)
			if err != nil {
				return nil, "", err
			}
			contents = append(contents, content)

		case llm.RoleTool:
			contents = append(contents, toGeminiFunctionResponseContent(msg))
		}
	}

	return contents, systemInstruction, nil
}

// toGeminiUserMultimodal converts a multimodal message into a Gemini user
// content with typed parts.
func (c *GeminiClient) toGeminiUserMultimodal(msg llm.Message) (*genai.Content, error) {
	parts := make([]*genai.Part, 0, len(msg.Parts))

	for _, part := range msg.Parts {
		switch part.Type {
		case llm.ContentPartTypeText:
			parts = append(parts, &genai.Part{Text: part.Text})

		case llm.ContentPartTypeImage:
			if !c.capabilities.SupportsVision {
				parts = append(parts, &genai.Part{
					Text: fmt.Sprintf("[Image attached: %s — this model does not support vision]", part.Filename),
				})
				continue
			}
			parts = append(parts, &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: part.MIMEType,
					Data:     part.Data,
				},
			})

		case llm.ContentPartTypeDocument:
			if part.MIMEType == "application/pdf" {
				// Gemini supports PDFs natively as inline blobs.
				parts = append(parts, &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: "application/pdf",
						Data:     part.Data,
					},
				})
			} else if text, ok := media.ExtractText(part.Data); ok {
				// Text-based formats — inline as text.
				parts = append(parts, &genai.Part{Text: text})
			} else {
				parts = append(parts, &genai.Part{
					Text: fmt.Sprintf("[Attached file: %s (%s) — binary format not supported]",
						part.Filename, part.MIMEType),
				})
			}
		}
	}

	return &genai.Content{
		Role:  "user",
		Parts: parts,
	}, nil
}

// toGeminiAssistantContent converts an assistant message into a Gemini content block
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
		// Decode our string ID back into the original []byte ThoughtSignature
		sum := sha256.Sum256([]byte(tc.ID))
		sigBytes := sum[:]

		parts = append(parts, &genai.Part{
			ThoughtSignature: sigBytes,
			FunctionCall: &genai.FunctionCall{
				Name: tc.Function.Name,
				Args: args,
				ID:   tc.ID,
			},
		})
	}

	// (Assuming genai.NewContentFromParts exists in your SDK version,
	// otherwise build the &genai.Content{} manually like you did before)
	return genai.NewContentFromParts(parts, "model"), nil
}

// toGeminiFunctionResponseContent wraps a tool result message
func toGeminiFunctionResponseContent(msg llm.Message) *genai.Content {
	return &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{
				FunctionResponse: &genai.FunctionResponse{
					Name: msg.Name,
					ID:   msg.ToolCallID, // This must match the Part.ThoughtSignature used above
					Response: map[string]any{
						"result": msg.Content,
					},
				},
			},
		},
	}
}

// toGeminiTools converts tool definitions to Gemini format
func toGeminiTools(tools []tools.ToolDef) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}

	decls := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		decls = append(decls, &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  toGeminiSchema(t.Parameters),
		})
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// toGeminiSchema converts JSON Schema to Gemini Schema
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

// contentsToPartsForChat converts Contents to Parts for chat streaming
func contentsToPartsForChat(contents []*genai.Content) []genai.Part {
	var parts []genai.Part
	for _, content := range contents {
		if content != nil {
			for _, part := range content.Parts {
				parts = append(parts, *part)
			}
		}
	}
	return parts
}

// parseResponse converts a Gemini response to our format
func parseResponse(resp *genai.GenerateContentResponse) (*llm.Response, error) {
	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: no candidates in response")
	}

	candidate := resp.Candidates[0]
	var textBuilder strings.Builder
	var toolCalls []tools.ToolCall

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

				// If this specific Part has a ThoughtSignature, encode it as our ID
				if len(part.ThoughtSignature) > 0 {
					tc.ID = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
				}
				toolCalls = append(toolCalls, tc)
			}

		}
	}

	return &llm.Response{
		Content:    textBuilder.String(),
		ToolCalls:  toolCalls,
		StopReason: string(candidate.FinishReason),
		Usage:      extractUsage(resp.UsageMetadata),
		Model:      resp.ModelVersion,
	}, nil
}

// fromGeminiFunctionCall converts a Gemini function call to our format
func fromGeminiFunctionCall(fc *genai.FunctionCall) (tools.ToolCall, error) {
	argsJSON, err := json.Marshal(fc.Args)
	if err != nil {
		return tools.ToolCall{}, fmt.Errorf("gemini: failed to marshal function call args: %w", err)
	}

	// Capture the ACTUAL ID/Thought Signature from Gemini.
	// (Check your genai SDK, this field may be called 'Id' or 'ThoughtSignature' in your version)
	id := fc.ID

	// Keep the fallback just in case, but we want the real one to pass through!
	if id == "" {
		id = fmt.Sprintf("%s_%d", fc.Name, time.Now().UnixNano())
	}

	return tools.ToolCall{
		ID:   id, // Now storing the real Gemini thought signature here
		Type: "function",
		Function: tools.FunctionCall{
			Name:      fc.Name,
			Arguments: string(argsJSON),
		},
	}, nil
}

// extractUsage extracts token counts from response metadata
func extractUsage(meta *genai.GenerateContentResponseUsageMetadata) llm.Usage {
	if meta == nil {
		return llm.Usage{}
	}
	return llm.Usage{
		InputTokens:  int(meta.PromptTokenCount),
		OutputTokens: int(meta.CandidatesTokenCount),
		TotalTokens:  int(meta.TotalTokenCount),
		CachedTokens: int(meta.CachedContentTokenCount), // This is the cached token count!
	}
}

// parseArgsToMap deserializes JSON arguments to a map
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
