package tools

// WebSearchInput is the input schema for the websearch_search tool.
type WebSearchInput struct {
	Query      string `json:"query" desc:"Search query (what to search for)"`
	MaxResults int    `json:"max_results,omitempty" desc:"Number of results to return (1-10, default: 5)"`
}

func GetWebSearchTools() []ToolDef {
	return []ToolDef{
		NewToolDef(
			SkillWebSearch,
			"websearch_search",
			"Search the web for current information. Returns titles, URLs, and snippets from search results.",
			SchemaFromStruct(WebSearchInput{}),
		),
	}
}
