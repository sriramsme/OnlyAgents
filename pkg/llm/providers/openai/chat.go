package openai

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// Chat sends a non-streaming chat completion request
func (c *OpenAIClient) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	start := time.Now()

	// Runtime validation
	if len(req.Tools) > 0 && !c.Capabilities().SupportsToolCalling {
		return nil, fmt.Errorf("model %s does not support tool calling", c.model)
	}

	params := c.buildChatParams(req)

	logger.Log.Debug("openai request",
		"model", c.model,
		"messages", len(req.Messages),
		"tools", len(req.Tools))

	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}
	for stream.Next() {
		acc.AddChunk(stream.Current())
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("openai api error: %w", err)
	}

	resp := parseResponse(&acc.ChatCompletion)

	logger.Log.Info("openai response",
		"model", c.model,
		"prompt_tokens", resp.Usage.InputTokens,
		"completion_tokens", resp.Usage.OutputTokens,
		// "cached_tokens", resp.Usage.CachedTokens,
		"has_tool_calls", resp.HasToolCalls(),
		"latency_ms", time.Since(start).Milliseconds())

	return resp, nil
}

// ChatStream sends a streaming chat completion request
func (c *OpenAIClient) ChatStream(ctx context.Context, req *llm.Request) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)

	go func() {
		defer close(ch)

		// Capability checks
		if !c.Capabilities().SupportsStreaming {
			ch <- llm.StreamChunk{
				Error: fmt.Errorf("model %s does not support streaming", c.model),
				Done:  true,
			}
			return
		}

		if len(req.Tools) > 0 && !c.Capabilities().SupportsToolCalling {
			ch <- llm.StreamChunk{
				Error: fmt.Errorf("model %s does not support tool calling", c.model),
				Done:  true,
			}
			return
		}

		start := time.Now()
		params := c.buildChatParams(req)

		logger.Log.Debug("openai streaming request",
			"model", c.model,
			"messages", len(req.Messages),
			"tools", len(req.Tools),
			"streaming", true)

		stream := c.client.Chat.Completions.NewStreaming(ctx, params)
		acc := openai.ChatCompletionAccumulator{}

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			// Check for finished content
			if _, ok := acc.JustFinishedContent(); ok {
				// Content stream finished
				logger.Log.Debug("openai streaming complete",
					"model", c.model,
					"total_tokens", acc.Usage.TotalTokens,
					"latency_ms", time.Since(start).Milliseconds())
			}

			// Check for finished tool call
			if tool, ok := acc.JustFinishedToolCall(); ok {
				// Convert to our format
				tc := tools.ToolCall{
					ID:   tool.ID,
					Type: "function",
					Function: tools.FunctionCall{
						Name:      tool.Name,
						Arguments: tool.Arguments,
					},
				}
				ch <- llm.StreamChunk{ToolCalls: []tools.ToolCall{tc}}
			}

			// Send content delta
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				ch <- llm.StreamChunk{Content: chunk.Choices[0].Delta.Content}
			}
		}

		if err := stream.Err(); err != nil && err != io.EOF {
			logger.Log.Error("stream error", "error", err)
			ch <- llm.StreamChunk{Error: err, Done: true}
		} else {
			ch <- llm.StreamChunk{Done: true}
		}

		logger.Log.Info("openai streaming complete",
			"model", c.model,
			"total_tokens", acc.Usage.TotalTokens,
			"latency_ms", time.Since(start).Milliseconds())
	}()

	return ch
}
