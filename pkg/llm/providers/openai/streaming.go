// Package llm provides LLM client abstractions for OnlyAgents
// This file contains the streaming version of the OpenAI provider
package openai

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// OpenAIStreamingClient implements Client for OpenAI with streaming support
type OpenAIStreamingClient struct {
	*OpenAIClient // Embed the basic client
}

// NewOpenAIStreamingClient creates a new OpenAI client with streaming
func NewOpenAIStreamingClient(cfg llm.ProviderConfig) (*OpenAIStreamingClient, error) {
	baseClient, err := NewOpenAIClient(cfg)
	if err != nil {
		return nil, err
	}

	return &OpenAIStreamingClient{
		OpenAIClient: baseClient,
	}, nil
}

// Chat sends a streaming chat completion request to OpenAI
func (c *OpenAIStreamingClient) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	start := time.Now()

	// Convert messages to OpenAI format
	messages := c.toOpenAIMessages(req.Messages)

	// Convert tools to OpenAI format
	tools := c.toOpenAITools(req.Tools)

	logger.Log.Debug("openai streaming request",
		"model", c.model,
		"messages", len(req.Messages),
		"tools", len(req.Tools))

	// Build request
	chatReq := openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Stream:      true, // Enable streaming
		MaxTokens:   c.getMaxTokens(req),
		Temperature: float32(c.getTemperature(req)),
	}

	// Add tools if provided
	if len(tools) > 0 {
		chatReq.Tools = tools
		chatReq.ToolChoice = "auto"
	}

	// Create streaming request
	stream, err := c.client.CreateChatCompletionStream(ctx, chatReq)
	if err != nil {
		logger.Log.Error("openai streaming error",
			"model", c.model,
			"error", err)
		return nil, fmt.Errorf("openai streaming error: %w", err)
	}
	defer func() {
		if cerr := stream.Close(); cerr != nil {
			logger.Log.Error("openai streaming close error",
				"model", c.model,
				"error", cerr)
		}
	}()

	// Accumulate response from stream
	resp, err := c.accumulateStream(stream)
	if err != nil {
		return nil, err
	}

	logger.Log.Info("openai streaming response",
		"model", c.model,
		"prompt_tokens", resp.Usage.InputTokens,
		"completion_tokens", resp.Usage.OutputTokens,
		"total_tokens", resp.Usage.TotalTokens,
		"has_tool_calls", resp.HasToolCalls(),
		"latency_ms", time.Since(start).Milliseconds())

	return resp, nil
}

// accumulateStream reads all chunks from the stream and assembles the response
func (c *OpenAIStreamingClient) accumulateStream(stream *openai.ChatCompletionStream) (*llm.Response, error) {
	var contentBuilder strings.Builder
	toolCallsMap := make(map[int]*llm.ToolCall) // Index -> ToolCall
	var finishReason string
	var usage llm.Usage

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Log.Error("stream recv error", "error", err)
			return nil, fmt.Errorf("stream recv error: %w", err)
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		// Accumulate content
		if choice.Delta.Content != "" {
			contentBuilder.WriteString(choice.Delta.Content)
		}

		// Accumulate tool calls
		if len(choice.Delta.ToolCalls) > 0 {
			for _, tc := range choice.Delta.ToolCalls {

				if tc.Index == nil {
					continue
				}
				index := *tc.Index

				// Initialize tool call if not exists
				if _, exists := toolCallsMap[index]; !exists {
					toolCallsMap[index] = &llm.ToolCall{
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "",
							Arguments: "",
						},
					}
				}

				// Update tool call
				if tc.ID != "" {
					toolCallsMap[index].ID = tc.ID
				}
				if tc.Function.Name != "" {
					toolCallsMap[index].Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					toolCallsMap[index].Function.Arguments += tc.Function.Arguments
				}
			}
		}

		// Capture finish reason
		if choice.FinishReason != "" {
			finishReason = string(choice.FinishReason)
		}

		// Usage info (typically in last chunk)
		if chunk.Usage != nil {
			usage = llm.Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
				TotalTokens:  chunk.Usage.TotalTokens,
			}
		}
	}

	// Convert toolCallsMap to slice
	var toolCalls []llm.ToolCall
	if len(toolCallsMap) > 0 {
		// Sort by index and extract
		maxIndex := 0
		for idx := range toolCallsMap {
			if idx > maxIndex {
				maxIndex = idx
			}
		}

		toolCalls = make([]llm.ToolCall, 0, len(toolCallsMap))
		for i := 0; i <= maxIndex; i++ {
			if tc, exists := toolCallsMap[i]; exists {
				toolCalls = append(toolCalls, *tc)
			}
		}
	}

	return &llm.Response{
		Content:    contentBuilder.String(),
		ToolCalls:  toolCalls,
		StopReason: finishReason,
		Usage:      usage,
		Model:      c.model,
	}, nil
}

// getMaxTokens returns the appropriate max tokens value
func (c *OpenAIClient) getMaxTokens(req *llm.Request) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	return c.maxTokens
}

// getTemperature returns the appropriate temperature value
func (c *OpenAIClient) getTemperature(req *llm.Request) float64 {
	if req.Temperature > 0 {
		return req.Temperature
	}
	return c.temperature
}

// ChatStream provides a streaming interface (future enhancement)
// This would be used like:
// stream := client.ChatStream(ctx, req)
//
//	for chunk := range stream {
//	    fmt.Print(chunk.Content)
//	}
type StreamChunk struct {
	Content   string
	ToolCalls []llm.ToolCall
	Done      bool
	Error     error
}

// ChatStream streams responses (future API extension)
func (c *OpenAIStreamingClient) ChatStream(ctx context.Context, req *llm.Request) <-chan StreamChunk {
	ch := make(chan StreamChunk)

	go func() {
		defer close(ch)

		// Convert messages and tools
		messages := c.toOpenAIMessages(req.Messages)
		tools := c.toOpenAITools(req.Tools)

		chatReq := openai.ChatCompletionRequest{
			Model:       c.model,
			Messages:    messages,
			Stream:      true,
			MaxTokens:   c.getMaxTokens(req),
			Temperature: float32(c.getTemperature(req)),
		}

		if len(tools) > 0 {
			chatReq.Tools = tools
			chatReq.ToolChoice = "auto"
		}

		stream, err := c.client.CreateChatCompletionStream(ctx, chatReq)
		if err != nil {
			ch <- StreamChunk{Error: err, Done: true}
			return
		}
		defer func() {
			if cerr := stream.Close(); cerr != nil {
				logger.Log.Error("openai streaming close error",
					"model", c.model,
					"error", cerr)
			}
		}()

		toolCallsMap := make(map[int]*llm.ToolCall)

		for {
			chunk, err := stream.Recv()
			if err == io.EOF {
				ch <- StreamChunk{Done: true}
				break
			}
			if err != nil {
				ch <- StreamChunk{Error: err, Done: true}
				break
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]

			// Send content chunks
			if choice.Delta.Content != "" {
				ch <- StreamChunk{
					Content: choice.Delta.Content,
					Done:    false,
				}
			}

			// Accumulate tool calls
			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					if tc.Index == nil {
						continue
					}
					index := *tc.Index
					if _, exists := toolCallsMap[index]; !exists {
						toolCallsMap[index] = &llm.ToolCall{
							Type:     "function",
							Function: llm.FunctionCall{},
						}
					}

					if tc.ID != "" {
						toolCallsMap[index].ID = tc.ID
					}
					if tc.Function.Name != "" {
						toolCallsMap[index].Function.Name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						toolCallsMap[index].Function.Arguments += tc.Function.Arguments
					}
				}
			}

			// Send complete tool calls when done
			if choice.FinishReason == openai.FinishReasonToolCalls {
				var toolCalls []llm.ToolCall
				for i := 0; i < len(toolCallsMap); i++ {
					if tc, exists := toolCallsMap[i]; exists {
						toolCalls = append(toolCalls, *tc)
					}
				}
				ch <- StreamChunk{
					ToolCalls: toolCalls,
					Done:      true,
				}
			}
		}
	}()

	return ch
}
