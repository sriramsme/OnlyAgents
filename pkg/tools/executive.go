package tools

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/core"
)

type AgentInfo struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Capabilities []core.Capability `json:"capabilities"`
}

type FindBestAgentFunc func(ctx context.Context, task string, capabilities []core.Capability) (AgentInfo, error)

// ====================
// Input Types
// ====================

// DelegateInput is the input schema for the delegate_to_agent tool.
type DelegateInput struct {
	AgentID      string            `json:"agent_id" desc:"ID of the agent to delegate to — use the 'id' field from the Available Capabilities & Agents section"`
	Task         string            `json:"task" desc:"Clear description of the task to delegate"`
	Capabilities []core.Capability `json:"capabilities,omitempty" desc:"Required capabilities for this task (for validation)"`
	Context      map[string]any    `json:"context,omitempty" desc:"Additional context for the delegated task (optional)"`

	// SendDirectlyToUser controls response routing:
	// - true: Sub-agent sends response directly to user (faster, for simple requests)
	// - false (default): Sub-agent returns to executive for synthesis/further processing
	// Use true when:
	//   - User asked a single question requiring one capability
	//   - No further processing needed by executive
	//   - Response can go directly to user
	// Use false when:
	//   - Executive needs the response for multi-step orchestration
	//   - Response will be used as context for next delegation
	//   - Executive needs to synthesize results from multiple agents
	SendDirectlyToUser bool `json:"send_directly_to_user,omitempty" desc:"If true, sub-agent sends response directly to user. If false (default), returns to executive for synthesis."`
}

// CreateWorkflowInput is the input schema for the create_workflow tool.
type CreateWorkflowInput struct {
	Name  string         `json:"name" desc:"Name for this workflow"`
	Tasks []WorkflowTask `json:"tasks" desc:"List of tasks in the workflow"`
}

// WorkflowTask defines a single task within a workflow.
type WorkflowTask struct {
	ID                   string   `json:"id" desc:"Unique task identifier (e.g. task_1, task_2)"`
	Name                 string   `json:"name" desc:"Short task name"`
	Description          string   `json:"description" desc:"Clear description of what this task should do"`
	RequiredCapabilities []string `json:"required_capabilities" desc:"Capabilities required to execute this task"`
	DependsOn            []string `json:"depends_on,omitempty" desc:"IDs of tasks that must complete before this one"`
}

// FindBestAgentInput is the input schema for the find_best_agent tool.
type FindBestAgentInput struct {
	Task         string            `json:"task" desc:"Clear description of the task to route"`
	Capabilities []core.Capability `json:"capabilities,omitempty" desc:"Required capabilities for this task"`
}

// ====================
// Tool Definitions
// ====================

// GetExecutiveTools returns orchestration tools for the executive agent.
// These are NOT regular skills — they trigger kernel routing events.
func GetExecutiveTools() []ToolDef {
	// Build capability-aware schemas by injecting enum values at construction time.
	// SchemaFromStruct handles structure; we patch in the capability enum after.
	delegateSchema := SchemaFromStruct(DelegateInput{})
	InjectEnumOnArrayField(delegateSchema, "capabilities", core.AllCapabilityStrings())

	workflowSchema := SchemaFromStruct(CreateWorkflowInput{})
	InjectEnumOnNestedArrayField(workflowSchema, "tasks", "required_capabilities", core.AllCapabilityStrings())

	findAgentSchema := SchemaFromStruct(FindBestAgentInput{})
	InjectEnumOnArrayField(findAgentSchema, "capabilities", core.AllCapabilityStrings())

	return []ToolDef{
		NewToolDef(
			"delegate_to_agent",
			"Delegate a task to a specialized agent. Use when a request requires specific capabilities "+
				"(calendar, email, web_search, etc.) that you don't handle directly. "+
				"Pick the agent_id from the Available Capabilities & Agents section in your context.\n\n"+
				"RESPONSE ROUTING:\n"+
				"- Set send_directly_to_user=true for simple, single-task requests where the sub-agent's response can go directly to the user\n"+
				"- Set send_directly_to_user=false (or omit) when you need the response for further processing or synthesis\n\n"+
				"Examples:\n"+
				"- 'Search for news on X' → send_directly_to_user=true (simple request, direct answer)\n"+
				"- 'Check my calendar' (as part of planning) → send_directly_to_user=false (you need this info to plan next steps)\n"+
				"- 'Send email to Bob' → send_directly_to_user=true (single action, user just needs confirmation)\n"+
				"- 'Get weather then suggest activities' → send_directly_to_user=false (you need weather data to suggest activities)",
			delegateSchema,
		),
		NewToolDef(
			"create_workflow",
			"Create a workflow with multiple interdependent tasks. Use when a request requires several steps "+
				"with dependencies (e.g. 'Check Bob's availability, then email him'). "+
				"Each task is delegated to an agent with matching capabilities. "+
				"Workflow results are ALWAYS returned to you for final synthesis - you cannot send task results directly to user.",
			workflowSchema,
		),
		NewToolDef(
			"find_best_agent",
			"Find the most suitable agent for a task based on required capabilities. "+
				"Use ONLY when no agent can be confidently selected from the Available Capabilities & Agents list. "+
				"If capabilities clearly match an agent, use delegate_to_agent directly instead.",
			findAgentSchema,
		),
	}
}
