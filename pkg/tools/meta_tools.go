package tools

// ActivateGroupsInput is the input schema for meta_activate_groups.
type ActivateGroupsInput struct {
	Groups map[string][]string `json:"groups" desc:"Map of skill name to list of group names to activate. Example: {\"git\": [\"inspect\", \"commit\"], \"github\": [\"pr_write\"]}"`
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
	}
}
