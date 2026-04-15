package tools

// ActivateGroupsInput is the input schema for meta_activate_groups.
type ActivateGroupsInput struct {
	Groups map[string][]string `json:"groups" desc:"Map of skill name to list of group names to activate. Example: {\"git\": [\"inspect\", \"commit\"], \"github\": [\"pr_write\"]}"`
}

// RememberInput is the schema for the remember tool.
type RememberInput struct {
	SubjectName string  `json:"subject_name" desc:"The entity this fact is about. e.g. 'user', 'Kai', 'Orion'"`
	SubjectType string  `json:"subject_type" desc:"only use one of these: person|project|tool|concept|decision|preference|other"`
	Predicate   string  `json:"predicate" desc:"Relationship verb. e.g. 'prefers', 'decided', 'uses', 'avoids'"`
	Object      string  `json:"object" desc:"The value or entity name. e.g. 'short responses', 'Postgres', 'Clerk'"`
	IsLiteral   bool    `json:"is_literal" desc:"true if object is a plain value, false if object is itself an entity"`
	Confidence  float32 `json:"confidence" desc:"How confident is this fact. 0.0-1.0. Use 1.0 for explicit statements."`
}

type RecallInput struct {
	Query string `json:"query" desc:"The user's current message or a natural language description of what context is needed."`
}

func recallTool() ToolDef {
	schema := SchemaFromStruct(RecallInput{})
	return NewToolDef(
		"meta_tools",
		"recall",
		"Search long-term memory for context about the user, their projects, "+
			"decisions, and preferences. Call this when the user references "+
			"something from a past conversation or when relevant context "+
			"would improve your response.",
		schema,
		"",
	)
}

func rememberTool() ToolDef {
	schema := SchemaFromStruct(RememberInput{})
	return NewToolDef(
		"meta_tools",
		"remember",
		"Store a fact, decision, or preference in long-term memory immediately. "+
			"Call this when the user explicitly asks you to remember something, "+
			"or when an important decision or preference is established that "+
			"should persist across conversations. "+
			"Use precise predicates: 'prefers', 'decided', 'uses', 'avoids', 'works_on'.",
		schema,
		"",
	)
}

// GetSubAgentMetaTools returns meta tools injected into every non-executive agent
// when group management is active (tool count exceeds threshold).
// These are handled internally by the agent — never routed to a skill or kernel.
func GetSubAgentMetaTools() []ToolDef {
	schema := SchemaFromStruct(ActivateGroupsInput{})

	return []ToolDef{
		NewToolDef(
			"meta_tools",
			"meta_activate_groups",
			"Select which tool groups to activate for this task. "+
				"After calling this, your next turn will include all tools from the selected groups.\n\n"+
				"Rules:\n"+
				"- Select the minimum groups needed for the task\n"+
				"- Include a skill's 'passthrough' group only if no named group covers the operation\n"+
				"- Check the 'Tool Groups' section of your system prompt for valid skill and group names\n\n"+
				"Example:\n"+
				"  {\"groups\": {\"git\": [\"inspect\", \"commit\"], \"github\": [\"pr_write\", \"ci\"]}}",
			schema,
			"", // no group — meta tools are not part of any skill group
		),
		recallTool(),
	}
}
