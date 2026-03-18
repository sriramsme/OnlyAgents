package tools

// ====================
// Input Types
// ====================

// DelegateInput is the input schema for the delegate_to_agent tool.
type DelegateInput struct {
	AgentID string         `json:"agent_id" desc:"ID of the agent to delegate to — use the 'id' field from the Available Sub-Agents & Their Capabilities section"`
	Task    string         `json:"task" desc:"Clear description of the task to delegate"`
	Context map[string]any `json:"context,omitempty" desc:"Additional context for the delegated task (optional)"`

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
	Goal  string         `json:"goal" desc:"Clear description of the goal of this workflow"`
	Steps []WorkflowStep `json:"steps" desc:"List of steps in the workflow"`
}

// WorkflowTask defines a single task within a workflow.
type WorkflowStep struct {
	ID        string   `json:"id" desc:"Unique step identifier (e.g. step_1, step_2)"`
	Name      string   `json:"name" desc:"Short name for this step"`
	Task      string   `json:"description" desc:"Clear description of what this step should do"`
	AgentID   string   `json:"agent_id" desc:"ID of the agent capable to execute this step  — use the 'id' field from the AVAILABLE SUB-AGENTS & THEIR CAPABILITIES section"`
	DependsOn []string `json:"depends_on,omitempty" desc:"IDs of steps that must complete before this one"`
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

	workflowSchema := SchemaFromStruct(CreateWorkflowInput{})

	return []ToolDef{
		NewToolDef(
			"meta_tools",
			"delegate_to_agent",
			"Delegate a task to a specialized agent. Use when a request requires specific skills and capabilities "+
				"(calendar, email, web_search, etc.) that you don't handle directly. "+
				"Pass a clear, self-contained description of what the agent should do — "+
				"rewrite the user's request in your own words with full context if needed. "+
				"For multiple operations targeting the same agent, pass them all in one delegation — "+
				"sub-agents handle multi-step execution internally.\n"+
				"Pick the agent_id from the 'Available Sub-Agents & Their Capabilities' section in your context.\n\n"+
				"RESPONSE ROUTING:\n"+
				"- Set send_directly_to_user=true for simple, single-task requests where the sub-agent's response can go directly to the user\n"+
				"- Set send_directly_to_user=false (or omit) when you need the response for further processing or synthesis\n\n"+
				"Examples:\n"+
				"- 'Search for news on X' → send_directly_to_user=true (simple request, direct answer)\n"+
				"- 'Check my calendar' (as part of planning) → send_directly_to_user=false (you need this info to plan next steps)\n"+
				"- 'Send email to Bob' → send_directly_to_user=true (single action, user just needs confirmation)\n"+
				"- 'Get weather then suggest activities' → send_directly_to_user=false (you need weather data to suggest activities)",
			delegateSchema,
			"",
		),
		NewToolDef(
			"meta_tools",
			"create_workflow",
			"Create a multi-step workflow when a request requires coordination across DIFFERENT agents or capabilities. "+
				"Each step is delegated to an agent matching its capabilities. "+
				"Results from all steps return to you for final synthesis — you cannot send step results directly to user. "+
				"ONLY use this when operations span multiple agents. "+
				"Do NOT use for multiple operations on the same agent (e.g. creating 3 tasks = one delegation to Friday, not a workflow). "+
				"Do NOT use for sequential operations a single agent can handle internally.",
			workflowSchema,
			"",
		),
	}
}
