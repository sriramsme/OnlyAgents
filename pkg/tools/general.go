// / pkg/tools/general.go
package tools

import (
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// FindSkillInput is the input schema for the find_skill tool.
type FindSkillInput struct {
	Capability  core.Capability `json:"capability" desc:"The capability needed to complete the task"`
	Description string          `json:"description" desc:"Brief description of what you're trying to accomplish"`
}

// UseSkillToolInput is the input schema for the use_skill_tool tool.
type UseSkillToolInput struct {
	SkillName  string                 `json:"skill_name" desc:"Name of the skill to use (from find_skill result)"`
	ToolName   string                 `json:"tool_name" desc:"Name of the tool to execute"`
	Parameters map[string]interface{} `json:"parameters" desc:"Tool parameters as key-value pairs"`
}

func GetGeneralTools() []ToolDef {
	findSkillSchema := SchemaFromStruct(FindSkillInput{})
	InjectEnumOnArrayField(findSkillSchema, "capability", core.AllCapabilityStrings())

	return []ToolDef{
		NewToolDef(
			SkillMetaTools,
			"find_skill",
			"Discover and load a skill for a required capability. "+
				"Returns the skill's available tools that you can then use via use_skill_tool.\n\n"+
				"The system will:\n"+
				"1. Check if skill is already loaded (instant)\n"+
				"2. Search local skills if not loaded\n"+
				"3. Auto-download from marketplaces if needed\n"+
				"4. Return skill info + all available tools\n\n"+
				"Example:\n"+
				"find_skill(capability='web_search') → Returns: {skill_name: 'websearch', tools: [{name: 'search_web', params: ...}]}",
			findSkillSchema,
		),
		NewToolDef(
			SkillMetaTools,
			"use_skill_tool",
			"Execute a specific tool from a loaded skill. "+
				"Must call find_skill first to discover available tools.\n\n"+
				"Example:\n"+
				"use_skill_tool(skill_name='websearch', tool_name='search_web', parameters={'query': 'AI news', 'max_results': 5})",
			SchemaFromStruct(UseSkillToolInput{}),
		),
	}
}
