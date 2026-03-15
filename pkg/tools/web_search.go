package tools

// WebSearchInput is the input schema for the websearch_search tool.
type WebSearchInput struct {
	Query      string `json:"query" desc:"Search query (what to search for)"`
	MaxResults int    `json:"max_results,omitempty" desc:"Number of results to return (1-10, default: 5)"`
}
type WebSearchFetchInput struct {
	URL       string `json:"url" desc:"Full URL to fetch and extract text content from"`
	MaxLength int    `json:"max_length,omitempty" desc:"Max characters to return (default: 8000, max: 32000)"`
}

func GetWebSearchTools() []ToolDef {
	return []ToolDef{
		NewToolDef(
			"websearch",
			"websearch_search",
			"Search the web for current information. Returns titles, URLs, and snippets from search results.",
			SchemaFromStruct(WebSearchInput{}),
		),
		NewToolDef(
			"websearch",
			"websearch_fetch",
			"Fetch a URL and extract its readable text content. Use after websearch_search to read the full content of a result.",
			SchemaFromStruct(WebSearchFetchInput{}),
		),
	}
}
