package anthropic

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// Chat sends a non-streaming chat completion request
func (c *AnthropicClient) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	start := time.Now()

	// Runtime validation
	if len(req.Tools) > 0 && !c.Capabilities().SupportsToolCalling {
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

	stream := c.client.Messages.NewStreaming(ctx, *params)
	message := &anthropic.Message{}
	for stream.Next() {
		if err := message.Accumulate(stream.Current()); err != nil {
			return nil, fmt.Errorf("anthropic stream accumulate: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		logger.Log.Error("anthropic api error", "model", c.model, "error", err)
		return nil, fmt.Errorf("anthropic api error: %w", err)
	}
	resp := parseResponse(message)

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
