package gemini

import (
	"context"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// Chat sends a non-streaming chat completion request
func (c *GeminiClient) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	start := time.Now()

	// Runtime validation
	if len(req.Tools) > 0 && !c.Capabilities().SupportsToolCalling {
		return nil, fmt.Errorf("model %s does not support tool calling", c.model)
	}

	contents, systemInstruction, err := c.toGeminiContents(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to convert messages: %w", err)
	}

	config := c.buildGenerateConfig(req, systemInstruction, contents)

	logger.Log.Debug("gemini request",
		"model", c.model,
		"messages", len(req.Messages),
		"tools", len(req.Tools),
		"streaming", false,
		"cached_content", c.cachedContentName != "")

	resp, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
	if err != nil {
		logger.Log.Error("gemini api error", "model", c.model, "error", err)
		return nil, fmt.Errorf("gemini: generate content failed: %w", err)
	}

	result, err := parseResponse(resp)
	if err != nil {
		return nil, err
	}

	logger.Log.Info("gemini response",
		"model", c.model,
		"prompt_tokens", result.Usage.InputTokens,
		"completion_tokens", result.Usage.OutputTokens,
		"cached_tokens", result.Usage.CachedTokens,
		"has_tool_calls", result.HasToolCalls(),
		"latency_ms", time.Since(start).Milliseconds())

	return result, nil
}

// ChatStream sends a streaming chat completion request
func (c *GeminiClient) ChatStream(ctx context.Context, req *llm.Request) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)

	go func() {
		defer close(ch)

		// Validate capabilities
		if err := c.validateStreamingCapabilities(req); err != nil {
			ch <- llm.StreamChunk{Error: err, Done: true}
			return
		}

		start := time.Now()

		contents, systemInstruction, err := c.toGeminiContents(req.Messages)
		if err != nil {
			ch <- llm.StreamChunk{Error: fmt.Errorf("gemini: failed to convert messages: %w", err), Done: true}
			return
		}

		config := c.buildGenerateConfig(req, systemInstruction, contents)

		logger.Log.Debug("gemini streaming request",
			"model", c.model,
			"messages", len(req.Messages),
			"tools", len(req.Tools),
			"streaming", true,
			"cached_content", c.cachedContentName != "")

		// Create a chat session for streaming
		chat, err := c.client.Chats.Create(ctx, c.model, config, nil)
		if err != nil {
			ch <- llm.StreamChunk{Error: fmt.Errorf("gemini: failed to create chat: %w", err), Done: true}
			return
		}

		// Convert contents to parts for streaming
		parts := contentsToPartsForChat(contents)

		// Process streaming results
		metrics := processStreamingResults(ctx, chat, parts, ch)

		// Send completion signal
		ch <- llm.StreamChunk{Done: true}

		logger.Log.Info("gemini streaming complete",
			"model", c.model,
			"input_tokens", metrics.InputTokens,
			"output_tokens", metrics.OutputTokens,
			"cached_tokens", metrics.CachedTokens,
			"latency_ms", time.Since(start).Milliseconds())
	}()

	return ch
}
