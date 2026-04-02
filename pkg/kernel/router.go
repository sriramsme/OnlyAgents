package kernel

import (
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// nolint:gocyclo
func (k *Kernel) route(evt core.Event) {
	k.logger.Debug("routing event",
		"type", evt.Type,
		"correlation_id", evt.CorrelationID,
		"agent_id", evt.AgentID)

	switch evt.Type {
	// User-facing
	case core.MessageReceived:
		k.handleMessageReceived(evt)

	case core.OutboundMessage:
		k.handleOutboundMessage(evt)

	// Agent execution
	case core.AgentExecute:
		// This is typically handled directly by agents' inboxes
		// If it arrives here, route it
		k.handleAgentExecute(evt)

	// Delegation
	case core.AgentDelegate:
		k.handleAgentDelegate(evt)

	case core.DelegationResult:
		// Results typically go directly via ReplyTo channel
		k.logger.Debug("delegation result received",
			"correlation_id", evt.CorrelationID)

	// Workflow
	case core.WorkflowSubmitted:
		k.handleWorkflowSubmitted(evt)

	case core.WorkflowInstantiate:
		k.handleWorkflowInstantiate(evt)

	case core.WorkflowCompleted:
		k.handleWorkflowCompleted(evt)

	case core.TaskAssigned:
		k.handleTaskAssigned(evt)

	case core.TaskCompleted:
		k.handleTaskCompleted(evt)

	case core.SessionGet:
		k.handleSessionGet(evt)

	case core.SessionNew:
		k.handleSessionNew(evt)

	case core.SessionEnsure:
		k.handleSessionEnsure(evt)

	case core.SessionEnd:
		k.handleSessionEnd(evt)

	case core.CronJobScheduled:
		k.handleCronJobScheduled(evt)

	// Future
	case core.AgentMessage:
		k.handleAgentMessage(evt)

	case core.OutboundToken:
		k.handleOutboundToken(evt)

	default:
		k.logger.Warn("unhandled event type", "type", evt.Type)
	}
}
