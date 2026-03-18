package tools

type SummarizeLength string

const (
	SummarizeLengthShort  SummarizeLength = "short"
	SummarizeLengthMedium SummarizeLength = "medium"
	SummarizeLengthLong   SummarizeLength = "long"
	SummarizeLengthXL     SummarizeLength = "xl"
)

type SummarizeTextInput struct {
	Text     string          `json:"text"               desc:"Text content to summarize"                                                        cli_short:"t" cli_pos:"1" cli_req:"true"`
	Length   SummarizeLength `json:"length,omitempty"   desc:"Summary length (short ~900, medium ~1800, long ~4200, xl ~9000). Default: medium" cli_short:"l"`
	Language string          `json:"language,omitempty" desc:"Output language (default: match source)"                                          cli_short:"g" cli_help:"e.g. english, spanish, french"`
	Focus    string          `json:"focus,omitempty"    desc:"Optional focus for the summary"                                                   cli_short:"f" cli_help:"e.g. key findings, technical details, action items"`
}

type SummarizeURLInput struct {
	URL      string          `json:"url"                desc:"URL to fetch and summarize"                                                       cli_short:"u" cli_pos:"1" cli_req:"true"`
	Length   SummarizeLength `json:"length,omitempty"   desc:"Summary length (short, medium, long, xl). Default: medium"                       cli_short:"l"`
	Language string          `json:"language,omitempty" desc:"Output language (default: match source)"                                          cli_short:"g"`
	Focus    string          `json:"focus,omitempty"    desc:"Optional focus for the summary"                                                   cli_short:"f"`
}

type SummarizeFileInput struct {
	Path     string          `json:"path"               desc:"Path to local file (.txt, .md, .pdf)"                                             cli_short:"p" cli_pos:"1" cli_req:"true"`
	Length   SummarizeLength `json:"length,omitempty"   desc:"Summary length (short, medium, long, xl). Default: medium"                       cli_short:"l"`
	Language string          `json:"language,omitempty" desc:"Output language (default: match source)"                                          cli_short:"g"`
	Focus    string          `json:"focus,omitempty"    desc:"Optional focus for the summary"                                                   cli_short:"f"`
}

type SummarizeYouTubeInput struct {
	URL      string          `json:"url"                desc:"YouTube URL (youtube.com or youtu.be)"                                            cli_short:"u" cli_pos:"1" cli_req:"true"`
	Length   SummarizeLength `json:"length,omitempty"   desc:"Summary length (short, medium, long, xl). Default: medium"                       cli_short:"l"`
	Language string          `json:"language,omitempty" desc:"Output language (default: match source)"                                          cli_short:"g"`
}

const (
	SummarizeTransform ToolGroup = "summarize_transform"
)

func GetSummarizeGroups() map[ToolGroup]string {
	return map[ToolGroup]string{
		SummarizeTransform: "Transform content into concise summaries from various sources (text, URLs, files, videos)",
	}
}

func GetSummarizeEntries() []ToolEntry {
	return []ToolEntry{
		{
			NewToolDef(
				"summarize",
				"summarize_text",
				"Summarize raw text. Use when you already have the content.",
				SchemaFromStruct(SummarizeTextInput{}),
				SummarizeTransform,
			),
			SummarizeTextInput{},
		},
		{
			NewToolDef(
				"summarize",
				"summarize_url",
				"Fetch a URL and summarize its content. Works for articles, docs, blog posts.",
				SchemaFromStruct(SummarizeURLInput{}),
				SummarizeTransform,
			),
			SummarizeURLInput{},
		},
		{
			NewToolDef(
				"summarize",
				"summarize_file",
				"Read and summarize a local file. Supports .txt, .md, and .pdf.",
				SchemaFromStruct(SummarizeFileInput{}),
				SummarizeTransform,
			),
			SummarizeFileInput{},
		},
		{
			NewToolDef(
				"summarize",
				"summarize_youtube",
				"Extract transcript and summarize a YouTube video. Gracefully falls back if no transcript available.",
				SchemaFromStruct(SummarizeYouTubeInput{}),
				SummarizeTransform,
			),
			SummarizeYouTubeInput{},
		},
	}
}

// GetSummarizeTools derives from entries — no duplication
func GetSummarizeTools() []ToolDef {
	entries := GetSummarizeEntries()
	defs := make([]ToolDef, len(entries))
	for i, e := range entries {
		defs[i] = e.Def
	}
	return defs
}
