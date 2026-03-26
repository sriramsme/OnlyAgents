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

const (
	WebSearch ToolGroup = "websearch"
)

func GetWebSearchGroups() map[ToolGroup]string {
	return map[ToolGroup]string{
		WebSearch: "Search and retrieve information from the web, including result discovery and content extraction",
	}
}

func GetWebSearchEntries() []ToolEntry {
	return []ToolEntry{
		{
			NewToolDef(
				"websearch",
				"websearch_search",
				"Search the web for current information. Returns titles, URLs, and snippets. "+
					"Each search should explore a distinct angle — do not repeat or slightly rephrase "+
					"a query that already returned results. If a search yields nothing useful, try ONE "+
					"alternative with genuinely different terms, then stop and work with what you have.",
				SchemaFromStruct(WebSearchInput{}),
				WebSearch,
			),
			WebSearchInput{},
		},
		{
			NewToolDef(
				"websearch",
				"websearch_fetch",
				"Fetch a URL and extract its full raw text content. Use only when you need verbatim "+
					"content: structured data, code, or exact quotes. For general reading and research, "+
					"prefer tools like summarize_url instead if available — it fetches and compresses in one step without "+
					"bloating context.",
				SchemaFromStruct(WebSearchFetchInput{}),
				WebSearch,
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
