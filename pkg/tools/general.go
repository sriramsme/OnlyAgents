// / pkg/tools/general.go
package tools

// FindSkillInput is the input schema for the find_skill tool.
type FindSkillInput struct {
	SkillName   string `json:"skill_name" desc:"The skill needed to complete the task"`
	Description string `json:"description" desc:"Brief description of what you're trying to accomplish"`
}

func GetGeneralTools() []ToolDef {
	return []ToolDef{
		NewToolDef(
			"meta_tools",
			"find_skill",
			`Discover and load a skill by name. Once loaded, the skill's tools are
immediately available for you to call directly in subsequent steps.

When to use: you need a capability not in your current toolset.

Example: find_skill("websearch") → tools added: [search_web, fetch_page]
You can then call search_web(...) directly.`,
			SchemaFromStruct(FindSkillInput{}),
		),
	}
}
