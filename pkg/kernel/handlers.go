package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

// ====================
// Executive Delegation System
// ====================

// ====================
// Message Flow Handlers
// ====================

// handleMessageReceived: ALL user messages go to executive first
func (k *Kernel) handleMessageReceived(evt core.Event) {
	payload, ok := evt.Payload.(core.MessageReceivedPayload)
	if !ok {
		k.logger.Error("invalid MessageReceived payload")
		return
	}

	// Get executive agent (entry point for all user messages)
	executive := k.agents.GetExecutive()

	correlationID := evt.CorrelationID
	if correlationID == "" {
		correlationID = uuid.NewString()
	}
	logger.Timing.StartPhase(correlationID, "end_to_end")
	logger.Timing.StartPhase(correlationID, "executive_routing")

	// Create agent execution event
	agentEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: correlationID,
		AgentID:       executive.ID(),
		Payload: core.AgentExecutePayload{
			Message:     payload.Content,
			MessageType: core.MessageTypeUser,
			Channel: &core.ChannelMetadata{
				SessionID: payload.Channel.SessionID,
				ChatID:    payload.Channel.ChatID,
				Name:      payload.Channel.Name,
				UserID:    payload.Channel.UserID,
				Username:  payload.Channel.Username,
			},
		},
	}

	// Send to executive
	select {
	case executive.Inbox() <- agentEvent:
		logger.Timing.EndPhase(correlationID, "executive_routing")
		k.logger.Debug("message routed to executive",
			"correlation_id", correlationID,
			"executive_id", executive.ID())

	case <-time.After(5 * time.Second):
		logger.Timing.EndPhase(correlationID, "executive_routing")
		k.logger.Error("executive inbox full - message dropped",
			"correlation_id", correlationID)
		// TODO: Send error response back to channel

	case <-k.ctx.Done():
		logger.Timing.EndPhase(correlationID, "executive_routing")
		k.logger.Info("shutdown in progress - message not delivered")
	}
}

// handleAgentDelegate: Executive wants to delegate a task
func (k *Kernel) handleAgentDelegate(evt core.Event) {
	payload, ok := evt.Payload.(core.AgentDelegatePayload)
	if !ok {
		k.logger.Error("invalid AgentDelegate payload")
		return
	}
	if k.uiBus != nil {
		k.uiBus <- core.UIEvent{
			Type:      core.UIEventDelegation,
			Timestamp: time.Now(),
			AgentID:   evt.AgentID,
			Payload: core.DelegationPayload{
				FromAgent: evt.AgentID,
				ToAgent:   payload.AgentID,
				Task:      payload.Task,
			},
		}
	}
	delegationPhase := fmt.Sprintf("delegation_%s", payload.AgentID)
	logger.Timing.StartPhase(evt.CorrelationID, delegationPhase)

	var targetAgent *agents.Agent
	var err error

	// Executive specifies agent_id directly (preferred)
	if payload.AgentID != "" {
		targetAgent, err = k.agents.Get(payload.AgentID)
		if err != nil {
			k.logger.Error("specified agent not found",
				"agent_id", payload.AgentID,
				"capabilities", payload.Capabilities)

			// Fallback: try to find agent by capabilities
			k.logger.Info("falling back to capability-based agent search")
			targetAgent = k.findBestAgent(payload.Capabilities, payload.Task)
		} else {
			// Validate agent has required capabilities (optional check)
			if len(payload.Capabilities) > 0 {
				agentCaps := k.getAgentCapabilities(targetAgent.GetSkillNames())
				if !hasAllCapabilities(agentCaps, payload.Capabilities) {
					k.logger.Warn("agent missing some capabilities",
						"agent_id", payload.AgentID,
						"has", agentCaps,
						"needs", payload.Capabilities)
					// Continue anyway - executive knows best
				}
			}
		}
	} else {
		// No agent_id specified - search by capabilities (fallback for non-executive callers)
		k.logger.Debug("no agent_id specified, searching by capabilities")
		targetAgent = k.findBestAgent(payload.Capabilities, payload.Task)
	}

	if targetAgent == nil {
		logger.Timing.EndPhase(evt.CorrelationID, delegationPhase)
		k.logger.Error("no agent found for delegation",
			"agent_id", payload.AgentID,
			"capabilities", payload.Capabilities)
		k.sendDelegationError(evt, fmt.Sprintf("No agent available for capabilities: %v", payload.Capabilities))
		return
	}

	k.logger.Info("delegating task",
		"from_agent", evt.AgentID,
		"to_agent", targetAgent.ID(),
		"capabilities", payload.Capabilities,
		"correlation_id", evt.CorrelationID)

	// Create execution event for target agent
	delegateEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: evt.CorrelationID,
		AgentID:       targetAgent.ID(),
		Payload: core.AgentExecutePayload{
			Message:     payload.Task,
			MessageType: core.MessageTypeDelegation,
			Channel:     payload.Channel,
			Delegation: &core.DelegationMetadata{
				DelegationID:       payload.DelegationID,
				SendDirectlyToUser: payload.SendDirectlyToUser,
				FromAgentID:        evt.AgentID,
				DelegatedAt:        time.Now(),
			},
		},
		ReplyTo: evt.ReplyTo, // Result goes back to delegating agent
	}

	// Send to target agent
	select {
	case targetAgent.Inbox() <- delegateEvent:
		k.logger.Debug("delegation sent",
			"to_agent", targetAgent.ID(),
			"correlation_id", evt.CorrelationID)
		// Note: We don't end the phase here - it ends when delegation completes

	case <-time.After(5 * time.Second):
		logger.Timing.EndPhase(evt.CorrelationID, delegationPhase)
		k.logger.Error("failed to delegate - agent inbox full",
			"agent_id", targetAgent.ID())
		k.sendDelegationError(evt, "Target agent busy")

	case <-k.ctx.Done():
		logger.Timing.EndPhase(evt.CorrelationID, delegationPhase)
		k.logger.Info("shutdown in progress - delegation not sent")
	}
}

// handleWorkflowSubmitted: Executive creates a multi-task workflow
func (k *Kernel) handleWorkflowSubmitted(evt core.Event) {
	payload, ok := evt.Payload.(workflow.WorkflowPayload)
	if !ok {
		k.logger.Error("invalid WorkflowSubmitted payload")
		return
	}
	workflowPhase := fmt.Sprintf("workflow_%s", payload.Workflow.ID)
	logger.Timing.StartPhase(evt.CorrelationID, workflowPhase)

	if k.workflow == nil {
		logger.Timing.EndPhase(evt.CorrelationID, workflowPhase)
		k.logger.Error("workflow engine not initialized")
		k.sendWorkflowError(evt, "Workflow engine unavailable")
		return
	}

	k.logger.Info("workflow submitted",
		"workflow_id", payload.Workflow.ID,
		"tasks", len(payload.Workflow.Tasks),
		"correlation_id", evt.CorrelationID)

	// Submit to workflow engine
	// Engine handles the DAG internally - no executive roundtrips per task
	if err := k.workflow.SubmitWorkflow(k.ctx, &payload.Workflow); err != nil {
		logger.Timing.EndPhase(evt.CorrelationID, workflowPhase)
		k.logger.Error("workflow submission failed",
			"workflow_id", payload.Workflow.ID,
			"error", err)
		k.sendWorkflowError(evt, err.Error())
		return
	}

	// Workflow engine now owns execution
	// It will fire WorkflowCompleted when ALL tasks done
	// Note: Phase ends when workflow completes
	k.logger.Debug("workflow accepted by engine",
		"workflow_id", payload.Workflow.ID)
}

// handleWorkflowCompleted: Workflow engine finished all tasks
func (k *Kernel) handleWorkflowCompleted(evt core.Event) {
	payload, ok := evt.Payload.(workflow.WorkflowResultPayload)
	if !ok {
		k.logger.Error("invalid WorkflowCompleted payload")
		return
	}

	workflowPhase := fmt.Sprintf("workflow_%s", payload.WorkflowID)
	logger.Timing.EndPhaseWithMetadata(evt.CorrelationID, workflowPhase, map[string]any{
		"status": payload.Status,
		"tasks":  len(payload.Results),
	})

	k.logger.Info("workflow completed",
		"workflow_id", payload.WorkflowID,
		"status", payload.Status,
		"tasks", len(payload.Results),
		"correlation_id", evt.CorrelationID)

	// Get the executive agent (who created the workflow)
	executive, err := k.agents.Get(payload.CreatedBy)
	if err != nil {
		k.logger.Error("executive agent not found", "agent_id", payload.CreatedBy)
		return
	}

	// Format workflow results into a prompt for executive to synthesize
	var message string
	if payload.Error != "" {
		message = fmt.Sprintf("The workflow (ID: %s) has failed with error: %s. Please inform the user.",
			payload.WorkflowID, payload.Error)
	} else {
		// Convert results to JSON for executive to process
		resultsJSON, err := json.Marshal(payload.Results)
		if err != nil {
			logger.Log.Warn("failed to marshal workflow results", "err", err)
			resultsJSON = []byte("failed to marshal results")
		}
		message = fmt.Sprintf("The user asked: \"%s\"\n\nYou created a workflow to handle this request. The workflow has completed successfully with the following task results:\n\n%s\n\nPlease synthesize these results into a coherent, natural response for the user that answers their original question.",
			payload.OriginalMessage, string(resultsJSON))
	}

	// Trigger executive to synthesize results
	synthesisEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: evt.CorrelationID,
		AgentID:       executive.ID(),
		Payload: core.AgentExecutePayload{
			Message:     message,
			MessageType: core.MessageTypeWorkflowCompleted,
			Channel:     payload.Channel,
		},
	}

	select {
	case executive.Inbox() <- synthesisEvent:
		k.logger.Debug("workflow synthesis task sent to executive")

	case <-time.After(5 * time.Second):
		k.logger.Error("failed to send workflow synthesis - executive inbox full",
			"workflow_id", payload.WorkflowID)

	case <-k.ctx.Done():
		k.logger.Info("shutdown in progress")
	}
}

// handleAgentExecute: Direct agent execution (rare - usually goes via inbox)
func (k *Kernel) handleAgentExecute(evt core.Event) {
	agent, err := k.agents.Get(evt.AgentID)
	if err != nil {
		k.logger.Error("target agent not found",
			"agent_id", evt.AgentID,
			"correlation_id", evt.CorrelationID)
		return
	}

	// Forward to agent's inbox
	select {
	case agent.Inbox() <- evt:
		k.logger.Debug("forwarded to agent", "agent_id", evt.AgentID)

	case <-time.After(5 * time.Second):
		k.logger.Error("agent inbox full",
			"agent_id", evt.AgentID,
			"correlation_id", evt.CorrelationID)

	case <-k.ctx.Done():
		return
	}
}

// handleTaskAssigned: Workflow engine assigned task to agent
func (k *Kernel) handleTaskAssigned(evt core.Event) {
	payload, ok := evt.Payload.(workflow.WFTaskAssignedPayload)
	if !ok {
		k.logger.Error("invalid TaskAssigned payload")
		return
	}

	// Get target agent (determined by workflow engine based on task capabilities)
	agent, err := k.agents.Get(evt.AgentID)
	if err != nil {
		k.logger.Error("agent not found for task assignment",
			"agent_id", evt.AgentID,
			"task_id", payload.TaskID)

		// TODO: Notify workflow engine of failure
		return
	}

	k.logger.Debug("assigning task to agent",
		"agent_id", agent.ID(),
		"task_id", payload.TaskID,
		"workflow_id", payload.WorkflowID,
		"channel", payload.Channel)

	// Create agent execute event
	agentEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: evt.CorrelationID,
		AgentID:       agent.ID(),
		Payload: core.AgentExecutePayload{
			Message:     payload.Task,
			MessageType: core.MessageTypeWorkflowTask,
			Workflow: &core.WorkflowMetadata{
				WorkflowID: payload.WorkflowID,
				TaskID:     payload.TaskID,
				TaskName:   payload.TaskName,
			},
			Channel: payload.Channel,
		},
		ReplyTo: evt.ReplyTo, // Result goes back to workflow engine
	}

	// Send to agent
	select {
	case agent.Inbox() <- agentEvent:
		k.logger.Debug("task assigned to agent",
			"agent_id", agent.ID(),
			"task_id", payload.TaskID)

	case <-time.After(5 * time.Second):
		k.logger.Error("failed to assign task - agent inbox full",
			"agent_id", agent.ID(),
			"task_id", payload.TaskID)
		// TODO: Notify workflow engine

	case <-k.ctx.Done():
		return
	}
}

func (k *Kernel) handleTaskCompleted(evt core.Event) {
	payload, ok := evt.Payload.(workflow.WFTaskCompletedPayload)
	if !ok {
		k.logger.Error("invalid TaskCompleted payload")
		return
	}

	k.logger.Debug("task completed",
		"workflow_id", payload.WorkflowID,
		"task_id", payload.TaskID,
		"has_error", payload.Error != "")

	if k.workflow == nil {
		k.logger.Error("workflow engine not initialized")
		return
	}

	// Pass to workflow engine
	if err := k.workflow.HandleTaskCompleted(k.ctx, payload); err != nil {
		k.logger.Error("failed to handle task completion",
			"error", err,
			"workflow_id", payload.WorkflowID,
			"task_id", payload.TaskID)
	}
}

// handleAgentMessage: Direct agent-to-agent communication (future)
func (k *Kernel) handleAgentMessage(evt core.Event) {
	payload, ok := evt.Payload.(core.AgentMessagePayload)
	if !ok {
		k.logger.Error("invalid AgentMessage payload")
		return
	}

	targetAgent, err := k.agents.Get(payload.ToAgent)
	if err != nil {
		k.logger.Error("target agent not found",
			"to_agent", payload.ToAgent,
			"from_agent", payload.FromAgent)
		return
	}

	k.logger.Debug("routing agent message",
		"from", payload.FromAgent,
		"to", payload.ToAgent)

	// Forward message
	agentEvent := core.Event{
		Type:          core.AgentExecute,
		CorrelationID: evt.CorrelationID,
		AgentID:       targetAgent.ID(),
		Payload: core.AgentExecutePayload{
			Message:     payload.Content,
			MessageType: core.MessageTypeAgentMessage,
			Agent: &core.AgentMetadata{
				FromAgent: payload.FromAgent,
			},
		},
	}

	select {
	case targetAgent.Inbox() <- agentEvent:
		k.logger.Debug("agent message delivered")

	case <-time.After(5 * time.Second):
		k.logger.Error("failed to deliver agent message - inbox full")

	case <-k.ctx.Done():
		return
	}
}

// handleToolCallRequest: agent wants to execute a tool, kernel dispatches to the right skill
func (k *Kernel) handleToolCallRequest(evt core.Event) {
	payload, ok := evt.Payload.(tools.ToolCallRequestPayload)
	if !ok {
		k.logger.Error("invalid ToolCallRequest payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload))
		return
	}

	skill, ok := k.skills.Get(payload.SkillName)
	if !ok {
		k.sendToolError(evt, fmt.Sprintf("skill not found: %s", payload.SkillName))
		return
	}

	// TRACKED GOROUTINE: Ensures graceful shutdown waits for tool calls
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()

		toolPhase := fmt.Sprintf("%s_tool_%s", evt.AgentID, payload.ToolName)
		logger.Timing.StartPhase(evt.CorrelationID, toolPhase)

		// Create timeout context for skill execution
		ctx, cancel := context.WithTimeout(k.ctx, 30*time.Second)
		defer cancel()

		k.logger.Debug("executing skill",
			"skill", payload.SkillName,
			"tool", payload.ToolName,
			"correlation_id", evt.CorrelationID)

		result, err := skill.Execute(ctx, payload.ToolName, payload.Arguments)

		metadata := map[string]any{
			"tool":  payload.ToolName,
			"skill": payload.SkillName,
		}
		if err != nil {
			metadata["error"] = "failed"
		}
		logger.Timing.EndPhaseWithMetadata(evt.CorrelationID, toolPhase, metadata)

		resultEvt := core.Event{
			Type:          core.ToolCallResult,
			CorrelationID: evt.CorrelationID,
			AgentID:       evt.AgentID,
		}

		if err != nil {
			k.logger.Error("skill execution failed",
				"skill", payload.SkillName,
				"tool", payload.ToolName,
				"error", err,
				"correlation_id", evt.CorrelationID)

			resultEvt.Payload = tools.ToolCallResultPayload{
				ToolCallID: payload.ToolCallID,
				ToolName:   payload.ToolName,
				Error:      err.Error(),
			}
		} else {
			k.logger.Debug("skill execution succeeded",
				"skill", payload.SkillName,
				"tool", payload.ToolName,
				"correlation_id", evt.CorrelationID)

			resultEvt.Payload = tools.ToolCallResultPayload{
				ToolCallID: payload.ToolCallID,
				ToolName:   payload.ToolName,
				Result:     result,
			}
		}

		// SAFE SEND: Reply directly to the agent's waiting goroutine
		if evt.ReplyTo != nil {
			select {
			case evt.ReplyTo <- resultEvt:
				// Success
			case <-time.After(5 * time.Second):
				k.logger.Error("failed to send tool result - reply channel blocked",
					"tool", payload.ToolName,
					"correlation_id", evt.CorrelationID,
					"warning", "agent may have timed out or shut down")
			case <-k.ctx.Done():
				k.logger.Info("shutdown in progress - tool result not delivered",
					"correlation_id", evt.CorrelationID)
			}
		} else {
			k.logger.Warn("tool call request missing ReplyTo channel",
				"tool", payload.ToolName,
				"correlation_id", evt.CorrelationID)
		}
	}()
}

// handleOutboundMessage: agent has a response, send it via the appropriate channel
func (k *Kernel) handleOutboundMessage(evt core.Event) {
	payload, ok := evt.Payload.(core.OutboundMessagePayload)
	if !ok {
		k.logger.Error("invalid OutboundMessage payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload))
		return
	}

	logger.Timing.StartPhase(evt.CorrelationID, "outbound_send")
	ch, err := k.channels.Get(payload.Channel.Name)
	if err != nil {
		logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
		k.logger.Error("channel not found",
			"channel", payload.Channel.Name,
			"correlation_id", evt.CorrelationID)
		return
	}
	// Create timeout context for channel send
	ctx, cancel := context.WithTimeout(k.ctx, 10*time.Second)
	defer cancel()

	if err := ch.Send(ctx, channels.OutgoingMessage{
		Channel:   payload.Channel,
		Content:   payload.Content,
		ReplyToID: payload.ReplyToID,
		ParseMode: payload.ParseMode,
	}); err != nil {
		logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
		k.logger.Error("failed to send outbound message",
			"channel", payload.Channel.Name,
			"correlation_id", evt.CorrelationID,
			"error", err)
	} else {
		logger.Timing.EndPhase(evt.CorrelationID, "outbound_send")
		k.logger.Debug("outbound message sent",
			"channel", payload.Channel.Name,
			"correlation_id", evt.CorrelationID)
	}

	logger.Timing.EndPhase(evt.CorrelationID, "end_to_end")
	logger.Timing.LogSummary(evt.CorrelationID)
}

// handleOutboundToken: Agent has a response, send it via the appropriate channel
func (k *Kernel) handleOutboundToken(evt core.Event) {
	payload, ok := evt.Payload.(core.OutboundTokenPayload)
	if !ok {
		k.logger.Error("invalid AgentToken payload",
			"actual_type", fmt.Sprintf("%T", evt.Payload))
		return
	}

	ch, err := k.channels.Get(payload.Channel.Name)
	if err != nil {
		k.logger.Error("channel not found for token",
			"channel", payload.Channel.Name,
			"correlation_id", evt.CorrelationID)
		return
	}

	streamer, ok := ch.(channels.TokenStreamer)
	if !ok {
		// Channel doesn't support streaming — silently skip,
		// final response still arrives via Send()
		return
	}

	ctx, cancel := context.WithTimeout(k.ctx, 5*time.Second)
	defer cancel()

	if err := streamer.SendToken(ctx, payload.Channel, payload.Token, payload.AccumulatedContent); err != nil {
		k.logger.Debug("send token failed",
			"channel", payload.Channel.Name,
			"error", err)
	}
}

// GetOrCreate — used by all channels on every message
func (k *Kernel) handleSessionGet(evt core.Event) {
	p := evt.Payload.(core.SessionGetPayload)
	sessionID, err := k.store.GetOrCreateSession(k.ctx, p.Channel, p.AgentID)
	if err != nil {
		k.logger.Error("session.get failed", "err", err)
		if evt.ReplyTo != nil {
			evt.ReplyTo <- core.Event{Payload: ""}
		}
		return
	}
	if evt.ReplyTo != nil {
		evt.ReplyTo <- core.Event{Payload: sessionID}
	}
}

// handleNewSession ends the current conversation and starts a fresh one.
// Triggered by a NewSession event, typically from a /newsession channel command.
func (k *Kernel) handleSessionNew(evt core.Event) {
	payload, ok := evt.Payload.(core.SessionNewPayload)
	if !ok {
		k.logger.Error("invalid NewSession payload")
		return
	}
	// End existing if any
	if existing, err := k.store.GetOrCreateSession(k.ctx, payload.Channel, payload.AgentID); err == nil {
		err := k.cm.EndSession(k.ctx, existing)
		if err != nil {
			k.logger.Error("failed to end existing session",
				"channel", payload.Channel,
				"session_id", existing,
				"err", err)
		}
	}

	sessionID, err := k.cm.NewSession(k.ctx, payload.Channel, payload.AgentID)
	if err != nil {
		k.logger.Error("failed to start new session",
			"channel", payload.Channel,
			"err", err)
		// Reply with empty string so caller doesn't hang
		if evt.ReplyTo != nil {
			evt.ReplyTo <- core.Event{Payload: ""}
		}
		return
	}

	k.logger.Info("new session started",
		"channel", payload.Channel,
		"session_id", sessionID)

	if evt.ReplyTo != nil {
		evt.ReplyTo <- core.Event{Payload: sessionID}
	}
}

func (k *Kernel) handleSessionEnd(evt core.Event) {
	payload, ok := evt.Payload.(core.SessionEndPayload)
	if !ok {
		k.logger.Error("invalid SessionEnd payload")
		return
	}

	if err := k.cm.EndSession(k.ctx, payload.SessionID); err != nil {
		k.logger.Warn("failed to end session",
			"session_id", payload.SessionID,
			"err", err)
		return
	}

	k.logger.Info("session ended", "session_id", payload.SessionID)
	// No ReplyTo needed — fire and forget
}

// sendToolError: Helper to send tool error back to agent
func (k *Kernel) sendToolError(evt core.Event, errorMsg string) {
	if evt.ReplyTo == nil {
		k.logger.Warn("cannot send tool error - no reply channel",
			"error", errorMsg)
		return
	}

	resultEvt := core.Event{
		Type:          core.ToolCallResult,
		CorrelationID: evt.CorrelationID,
		AgentID:       evt.AgentID,
		Payload: tools.ToolCallResultPayload{
			ToolCallID: "", // Extract from payload if available
			Error:      errorMsg,
		},
	}

	select {
	case evt.ReplyTo <- resultEvt:
		// Success
	case <-time.After(2 * time.Second):
		k.logger.Error("failed to send tool error")
	case <-k.ctx.Done():
		return
	}
}

// ====================
// Agent Finding Logic
// ====================

// findBestAgent finds the best agent for a task based on capabilities
func (k *Kernel) findBestAgent(capabilities []core.Capability, task string) *agents.Agent {
	// 1. Try to find specialized agent
	agent, _, found := k.findSpecializedAgent(capabilities)
	if found {
		return agent
	}

	// 2. Fall back to general agent
	generalAgent := k.agents.GetGeneral()

	return generalAgent
}

// findSpecializedAgent finds an agent that has skills for all required capabilities
func (k *Kernel) findSpecializedAgent(capabilities []core.Capability) (*agents.Agent, []core.Capability, bool) {
	for _, agent := range k.agents.All() {
		if agent.IsExecutive() {
			continue
		}

		// Check if agent has skills covering all capabilities
		agentCapabilities := k.getAgentCapabilities(agent.GetSkillNames())
		if hasAllCapabilities(agentCapabilities, capabilities) {
			return agent, capabilities, true
		}
	}
	return nil, nil, false
}

// ====================
// Error Handlers
// ====================

func (k *Kernel) sendDelegationError(evt core.Event, errorMsg string) {
	if evt.ReplyTo == nil {
		k.logger.Warn("cannot send delegation error - no reply channel")
		return
	}

	errorEvt := core.Event{
		Type:          core.DelegationResult,
		CorrelationID: evt.CorrelationID,
		Payload: core.DelegationResultPayload{
			Error: errorMsg,
		},
	}

	select {
	case evt.ReplyTo <- errorEvt:
		// Success
	case <-time.After(2 * time.Second):
		k.logger.Error("failed to send delegation error")
	case <-k.ctx.Done():
		return
	}
}

func (k *Kernel) sendWorkflowError(evt core.Event, errorMsg string) {
	// TODO: Send error back to executive
	k.logger.Error("workflow error", "error", errorMsg)
}

// ====================
// Capability Helpers
// ====================

func (k *Kernel) getAgentCapabilities(skillNames []tools.SkillName) []core.Capability {
	capSet := make(map[core.Capability]bool)

	for _, skillName := range skillNames {
		skill, ok := k.skills.Get(skillName)
		if !ok {
			continue
		}
		for _, cap := range skill.RequiredCapabilities() {
			capSet[cap] = true
		}
	}

	caps := make([]core.Capability, 0, len(capSet))
	for cap := range capSet {
		caps = append(caps, cap)
	}
	return caps
}

func hasAllCapabilities(agentCaps, required []core.Capability) bool {
	capMap := make(map[core.Capability]bool)
	for _, cap := range agentCaps {
		capMap[cap] = true
	}

	for _, req := range required {
		if !capMap[req] {
			return false
		}
	}
	return true
}
