// Agent has two interaction modes:
//
//  1. User↔Agent (synchronous): Execute() — called by HTTP handlers.
//     Returns a string response. Manages LLM loop internally.
//
//  2. Agent↔Agent (asynchronous): handleMessage() — driven by processMessages().
//     Messages are signed A2A protocol messages routed via incoming channel.
//     Never call handleMessage directly; send to incoming channel instead.

package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/a2a"
	"github.com/sriramsme/OnlyAgents/pkg/config"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/soul"
)

// NewAgent creates a new agent instance
func NewAgent(cfg config.Config, llmClient llm.Client) (*Agent, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("LLM client is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.With("agent_id", cfg.ID)

	agentSoul := soul.NewSoul(cfg.Soul)

	var userConfigPath string
	if cfg.UserRef == "" {
		userConfigPath = "configs/user.yaml"
	}

	userCfg, err := config.LoadUserConfig(userConfigPath)

	agent := &Agent{
		id:             cfg.ID,
		name:           cfg.Name,
		isExecutive:    cfg.IsExecutive,
		maxConcurrency: cfg.MaxConcurrency,
		bufferSize:     cfg.BufferSize,
		skills:         NewSkillRegistry(),
		state:          NewStateManager(),
		llmClient:      llmClient,
		incoming:       make(chan a2a.Message, cfg.BufferSize),
		outgoing:       make(chan a2a.Message, cfg.BufferSize),
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
		soul:           agentSoul,
		user:           userCfg,
		connectors: &ConnectorRegistry{ // Create empty registry
			connectors: make(map[string]Connector),
		},
	}

	return agent, err
}

// Start starts the agent
func (a *Agent) Start() error {
	a.logger.Info("starting agent",
		"provider", a.llmClient.Provider(),
		"model", a.llmClient.Model())

	// Start message processing
	a.wg.Add(1)
	go a.processMessages()

	// Start health check
	a.wg.Add(1)
	go a.healthCheck()

	a.logger.Info("agent started successfully")
	return nil
}

// Stop gracefully shuts down the agent
func (a *Agent) Stop() error {
	a.logger.Info("stopping agent")
	a.cancel()

	// Wait for goroutines to finish
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	// Wait with timeout
	select {
	case <-done:
		a.logger.Info("agent stopped successfully")
		return nil
	case <-time.After(time.Second * 5):
		a.logger.Error("agent stop timeout, forcefully shutting down")
		return fmt.Errorf("shutdown timeout")
	}
}

// processMessages is the main event loop of the agent
func (a *Agent) processMessages() {
	defer a.wg.Done()

	for {
		select {
		case msg := <-a.incoming:
			a.handleMessage(msg)
		case <-a.ctx.Done():
			return
		}
	}
}

// handleMessage is the agent-to-agent entry point.
// Called by processMessages for async A2A communication.
// Messages arrive signed, are verified, then dispatched to skills.
func (a *Agent) handleMessage(msg a2a.Message) {
	a.logger.Info("received message",
		"message_id", msg.ID,
		"from", msg.FromAgent,
		"action", msg.Action)

	// TODO: Full message handling pipeline
	// 1. Security verification
	// 2. Intent classification
	// 3. Skill selection and execution
	// 4. Response signing
}

// healthCheck periodically checks the agent's health
func (a *Agent) healthCheck() {
	defer a.wg.Done()

	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// TODO: Proper health checks
			a.logger.Debug("health check successful")
		case <-a.ctx.Done():
			return
		}
	}
}

// Execute is the user-facing entry point.
// Called by HTTP handlers for synchronous request/response.
// Manages the full LLM loop: prompt → tool calls → final response.
func (a *Agent) Execute(ctx context.Context, userMessage string) (string, error) {
	a.logger.Debug("executing user request",
		"message_length", len(userMessage))

	// Build conversation with system prompt
	messages := []llm.Message{
		llm.SystemMessage(a.soul.SystemPrompt(ctx)),
		llm.SystemMessage(a.formatUserProfile()),
		llm.UserMessage(userMessage),
	}

	// Get available skills as tools
	tools := a.skillsAsTools()

	// Call LLM with tool support
	resp, err := a.llmClient.Chat(ctx, &llm.Request{
		Messages: messages,
		Tools:    tools,
		Metadata: map[string]string{
			"agent_id": a.id,
		},
	})
	if err != nil {
		a.logger.Error("llm request failed", "error", err)
		return "", fmt.Errorf("llm request failed: %w", err)
	}

	// Log reasoning if available (extended thinking)
	if resp.ReasoningContent != "" {
		a.logger.Debug("agent reasoning available",
			"reasoning_length", len(resp.ReasoningContent))
		// Could store reasoning in state/memory for analysis
	}

	// Log token usage
	a.logger.Debug("llm response received",
		"input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens,
		"total_tokens", resp.Usage.TotalTokens,
		"stop_reason", resp.StopReason)

	// Handle tool calls if present
	if resp.HasToolCalls() {
		a.logger.Debug("executing tools",
			"tool_count", len(resp.ToolCalls))
		return a.handleToolCalls(ctx, messages, resp)
	}

	return resp.Content, nil
}

// handleToolCalls executes tools and continues the conversation
func (a *Agent) handleToolCalls(ctx context.Context, messages []llm.Message, resp *llm.Response) (string, error) {
	// Add assistant's response with tool calls
	messages = append(messages, llm.AssistantMessageWithTools(
		resp.Content,
		resp.ReasoningContent,
		resp.ToolCalls,
	))

	// Execute each tool
	for _, tc := range resp.ToolCalls {
		a.logger.Debug("executing tool",
			"tool", tc.Function.Name,
			"args", tc.Function.Arguments)

		// Parse arguments
		args, err := llm.ParseToolArguments(tc.Function.Arguments)
		if err != nil {
			a.logger.Error("failed to parse tool arguments",
				"tool", tc.Function.Name,
				"error", err)
			return "", fmt.Errorf("invalid tool arguments: %w", err)
		}

		// Find and execute skill
		skill, err := a.skills.Get(tc.Function.Name)
		if skill == nil {
			err := fmt.Errorf("skill not found: %s", tc.Function.Name)
			a.logger.Error("skill not found", "skill", tc.Function.Name)
			return "", err
		}
		if err != nil {
			a.logger.Error("skill issue", "skill", tc.Function.Name, "error", err)
			return "", err
		}

		// Execute skill
		result, err := skill.Execute(ctx, args)
		if err != nil {
			a.logger.Error("skill execution failed",
				"skill", tc.Function.Name,
				"error", err)
			return "", fmt.Errorf("skill execution failed: %w", err)
		}

		// Convert result to JSON string
		resultJSON, err := json.Marshal(result)
		if err != nil {
			a.logger.Error("failed to marshal skill result",
				"skill", tc.Function.Name,
				"error", err)
			return "", err
		}

		// Add tool result to conversation
		messages = append(messages, llm.ToolResultMessage(
			tc.ID,
			tc.Function.Name,
			string(resultJSON),
		))
	}

	// Continue conversation with tool results
	finalResp, err := a.llmClient.Chat(ctx, &llm.Request{
		Messages: messages,
		Tools:    a.skillsAsTools(),
		Metadata: map[string]string{
			"agent_id": a.id,
		},
	})
	if err != nil {
		return "", err
	}

	a.logger.Debug("final response after tools",
		"response_length", len(finalResp.Content),
		"tokens_used", finalResp.Usage.TotalTokens)

	return finalResp.Content, nil
}

// AskLLM is a helper method for skills to interact with LLM
// This provides a simple interface for skills that need LLM assistance
func (a *Agent) AskLLM(ctx context.Context, system, prompt string) (string, error) {
	if a.llmClient == nil {
		return "", fmt.Errorf("LLM client not configured")
	}

	messages := []llm.Message{
		llm.SystemMessage(system),
		llm.UserMessage(prompt),
	}

	resp, err := a.llmClient.Chat(ctx, &llm.Request{
		Messages: messages,
		Metadata: map[string]string{
			"agent_id": a.id,
			"context":  "skill_helper",
		},
	})
	if err != nil {
		return "", err
	}

	a.logger.Debug("llm helper response",
		"input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens,
		"model", resp.Model)

	return resp.Content, nil
}

// skillsAsTools converts registered skills to LLM tool definitions
func (a *Agent) skillsAsTools() []llm.ToolDef {
	skills := a.skills.GetAll()
	tools := make([]llm.ToolDef, 0, len(skills))

	for _, skill := range skills {
		tools = append(tools, llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        skill.Name(),
				Description: skill.Description(),
				Parameters:  skill.Parameters(),
			},
		})
	}

	return tools
}

// RegisterSkill registers a new skill with the agent
func (a *Agent) RegisterSkill(skill skills.Skill) error {
	a.logger.Info("registering skill",
		"skill", skill.Name())
	return a.skills.Register(skill)
}

// SendMessage sends a message to another agent (for A2A communication)
func (a *Agent) SendMessage(msg a2a.Message) error {
	select {
	case a.outgoing <- msg:
		return nil
	case <-time.After(time.Second * 1):
		return fmt.Errorf("send message timeout")
	}
}

// ReceiveMessage returns the incoming message channel (for A2A communication)
func (a *Agent) ReceiveMessage() <-chan a2a.Message {
	return a.incoming
}

// ID returns the agent's ID
func (a *Agent) ID() string {
	return a.id
}

// LLMClient returns the agent's LLM client (for advanced use cases)
func (a *Agent) LLMClient() llm.Client {
	return a.llmClient
}

// RegisterConnectors populates agent's connector registry
func (a *Agent) RegisterConnectors(connectorNames []string, globalRegistry *ConnectorRegistry) error {
	if len(connectorNames) == 0 {
		a.logger.Debug("no connectors configured for agent")
		return nil
	}

	var errs []error
	for _, name := range connectorNames {
		connector, err := globalRegistry.Get(name)
		if err != nil {
			errs = append(errs, fmt.Errorf("connector %s: %w", name, err))
			continue
		}

		// Add to agent's local registry
		a.connectors.mu.Lock()
		a.connectors.connectors[name] = connector
		a.connectors.mu.Unlock()

		a.logger.Info("connector registered", "connector", name)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to register connectors for agent %s: %v", a.id, errs)
	}

	return nil
}

// Delegate connector access methods to the registry
func (a *Agent) GetConnector(name string) (Connector, error) {
	return a.connectors.Get(name)
}

func (a *Agent) ListConnectors() []string {
	return a.connectors.List()
}

func (a *Agent) HasConnector(name string) bool {
	_, err := a.connectors.Get(name)
	return err == nil
}

func (a *Agent) formatUserProfile() string {
	return fmt.Sprintf(`

=== Who the user is ===

Name: %s (you prefer I call you "%s")
Job: %s

Background:
%s

Daily Routine:
%s

What the user values:
%s

User preferences:
- Technical: %v
- On collaboration: %s
	`,
		a.user.Identity.Name,
		a.user.Identity.PreferredName,
		a.user.Identity.Role,
		a.user.Background.Professional,
		a.user.DailyRoutine,
		strings.Join(a.user.Preferences.WhatIValue, "\n- "),
		a.user.Preferences.Technical,
		a.user.Preferences.Collaboration,
	)
}
