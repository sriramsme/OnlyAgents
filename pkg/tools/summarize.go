package tools

type SummarizeLength string

const (
	SummarizeLengthShort  SummarizeLength = "short"
	SummarizeLengthMedium SummarizeLength = "medium"
	SummarizeLengthLong   SummarizeLength = "long"
	SummarizeLengthXL     SummarizeLength = "xl"
)

type SummarizeTextInput struct {
	Text     string          `json:"text"               desc:"Text content to summarize"`
	Length   SummarizeLength `json:"length,omitempty"   desc:"short (~900), medium (~1800), long (~4200), xl (~9000). Default: medium"`
	Language string          `json:"language,omitempty" desc:"Output language, e.g. 'english'. Default: match source"`
	Focus    string          `json:"focus,omitempty"    desc:"Optional angle, e.g. 'key findings', 'technical details'"`
}

type SummarizeURLInput struct {
	URL      string          `json:"url"                desc:"Full URL to fetch and summarize"`
	Length   SummarizeLength `json:"length,omitempty"   desc:"short, medium, long, xl. Default: medium"`
	Language string          `json:"language,omitempty" desc:"Output language. Default: match source"`
	Focus    string          `json:"focus,omitempty"    desc:"Optional focus for the summary"`
}

type SummarizeFileInput struct {
	Path     string          `json:"path"               desc:"Local file path (.txt, .md, .pdf)"`
	Length   SummarizeLength `json:"length,omitempty"   desc:"short, medium, long, xl. Default: medium"`
	Language string          `json:"language,omitempty" desc:"Output language. Default: match source"`
	Focus    string          `json:"focus,omitempty"    desc:"Optional focus for the summary"`
}

type SummarizeYouTubeInput struct {
	URL      string          `json:"url"                desc:"YouTube URL (youtube.com or youtu.be)"`
	Length   SummarizeLength `json:"length,omitempty"   desc:"short, medium, long, xl. Default: medium"`
	Language string          `json:"language,omitempty" desc:"Output language. Default: match source"`
}

func GetSummarizeTools() []ToolDef {
	return []ToolDef{
		NewToolDef(
			"summarize",
			"summarize_text",
			"Summarize raw text. Use when you already have the content.",
			SchemaFromStruct(SummarizeTextInput{}),
		),
		NewToolDef(
			"summarize",
			"summarize_url",
			"Fetch a URL and summarize its content. Works for articles, docs, blog posts.",
			SchemaFromStruct(SummarizeURLInput{}),
		),
		NewToolDef(
			"summarize",
			"summarize_file",
			"Read and summarize a local file. Supports .txt, .md, and .pdf.",
			SchemaFromStruct(SummarizeFileInput{}),
		),
		NewToolDef(
			"summarize",
			"summarize_youtube",
			"Extract transcript and summarize a YouTube video. Gracefully falls back if no transcript available.",
			SchemaFromStruct(SummarizeYouTubeInput{}),
		),
	}
}
