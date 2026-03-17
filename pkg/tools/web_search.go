package tools

// WebSearchInput is the input schema for the websearch_search tool.
type WebSearchInput struct {
	Query      string `json:"query"              desc:"Search query (what to search for)"                 cli_short:"q" cli_pos:"1" cli_req:"true"`
	MaxResults int    `json:"max_results,omitempty" desc:"Number of results to return (1-10, default: 5)" cli_short:"n" cli_help:"e.g. 3, 5, 10"`
}

type WebSearchFetchInput struct {
	URL       string `json:"url"                 desc:"Full URL to fetch and extract text content from"   cli_short:"u" cli_pos:"1" cli_req:"true"`
	MaxLength int    `json:"max_length,omitempty" desc:"Max characters to return (default: 8000, max: 32000)" cli_short:"l" cli_help:"e.g. 4000, 8000, 16000"`
}

func GetWebSearchEntries() []ToolEntry {
	return []ToolEntry{
		{
			NewToolDef(
				"websearch",
				"websearch_search",
				"Search the web for current information. Returns titles, URLs, and snippets from search results.",
				SchemaFromStruct(WebSearchInput{}),
			),
			WebSearchInput{},
		},
		{
			NewToolDef(
				"websearch",
				"websearch_fetch",
				"Fetch a URL and extract its readable text content. Use after websearch_search to read the full content of a result.",
				SchemaFromStruct(WebSearchFetchInput{}),
			),
			WebSearchFetchInput{},
		},
	}
}

func GetWebSearchTools() []ToolDef {
	entries := GetWebSearchEntries()
	defs := make([]ToolDef, len(entries))
	for i, e := range entries {
		defs[i] = e.Def
	}
	return defs
}
