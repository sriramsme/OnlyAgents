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
	"github.com/sriramsme/OnlyAgents/pkg/tools"
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
	ToolCalls []tools.ToolCall
	Done      bool
	Error     error
}

func (c *OpenAIStreamingClient) ChatStream(ctx context.Context, req *llm.Request) <-chan StreamChunk {
	ch := make(chan StreamChunk)
	go func() {
		defer close(ch)

		stream, err := c.createStream(ctx, req)
		if err != nil {
			ch <- StreamChunk{Error: err, Done: true}
			return
		}
		defer func() {
			if cerr := stream.Close(); cerr != nil {
				logger.Log.Error("openai streaming close error", "model", c.model, "error", cerr)
			}
		}()

		c.processStream(stream, ch)
	}()
	return ch
}

// createStream builds the request and opens the stream
func (c *OpenAIStreamingClient) createStream(ctx context.Context, req *llm.Request) (*openai.ChatCompletionStream, error) {
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

	return c.client.CreateChatCompletionStream(ctx, chatReq)
}

// processStream reads chunks from the stream and sends them to the channel
func (c *OpenAIStreamingClient) processStream(stream *openai.ChatCompletionStream, ch chan<- StreamChunk) {
	toolCallsMap := make(map[int]*tools.ToolCall)

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			ch <- StreamChunk{Done: true}
			return
		}
		if err != nil {
			ch <- StreamChunk{Error: err, Done: true}
			return
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		if choice.Delta.Content != "" {
			ch <- StreamChunk{Content: choice.Delta.Content}
		}

		accumulateToolCalls(toolCallsMap, choice.Delta.ToolCalls)

		if choice.FinishReason == openai.FinishReasonToolCalls {
			ch <- StreamChunk{ToolCalls: collectToolCalls(toolCallsMap), Done: true}
			return
		}
	}
}

// accumulateToolCalls merges streaming tool call deltas into the map
func accumulateToolCalls(toolCallsMap map[int]*tools.ToolCall, deltas []openai.ToolCall) {
	for _, tc := range deltas {
		if tc.Index == nil {
			continue
		}
		index := *tc.Index
		if _, exists := toolCallsMap[index]; !exists {
			toolCallsMap[index] = &tools.ToolCall{Type: "function"}
		}
		entry := toolCallsMap[index]
		if tc.ID != "" {
			entry.ID = tc.ID
		}
		if tc.Function.Name != "" {
			entry.Function.Name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			entry.Function.Arguments += tc.Function.Arguments
		}
	}
}

// collectToolCalls converts the map into an ordered slice
func collectToolCalls(toolCallsMap map[int]*tools.ToolCall) []tools.ToolCall {
	toolCalls := make([]tools.ToolCall, 0, len(toolCallsMap))
	for i := 0; i < len(toolCallsMap); i++ {
		if tc, exists := toolCallsMap[i]; exists {
			toolCalls = append(toolCalls, *tc)
		}
	}
	return toolCalls
}
func (c *OpenAIStreamingClient) accumulateStream(stream *openai.ChatCompletionStream) (*llm.Response, error) {
	var contentBuilder strings.Builder
	toolCallsMap := make(map[int]*tools.ToolCall)
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
		contentBuilder.WriteString(choice.Delta.Content)
		accumulateToolCalls(toolCallsMap, choice.Delta.ToolCalls)

		if choice.FinishReason != "" {
			finishReason = string(choice.FinishReason)
		}
		if chunk.Usage != nil {
			usage = extractUsage(chunk.Usage)
		}
	}

	return &llm.Response{
		Content:    contentBuilder.String(),
		ToolCalls:  collectToolCalls(toolCallsMap),
		StopReason: finishReason,
		Usage:      usage,
		Model:      c.model,
	}, nil
}

// extractUsage converts openai usage to llm.Usage
func extractUsage(u *openai.Usage) llm.Usage {
	return llm.Usage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
		TotalTokens:  u.TotalTokens,
	}
}
