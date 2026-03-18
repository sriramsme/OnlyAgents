package tools

type NotesCreateInput struct {
	Notes []NotesCreateItem `json:"notes" desc:"One or more notes to create" cli_short:"n" cli_req:"true" cli_help:"JSON array or repeated input"`
}

type NotesCreateItem struct {
	Title   string   `json:"title"             desc:"Note title"                              cli_short:"t" cli_req:"true"`
	Content string   `json:"content,omitempty" desc:"Note body content in Markdown"          cli_short:"c" cli_help:"supports markdown"`
	Tags    []string `json:"tags,omitempty"    desc:"Tags to categorize the note"            cli_short:"g" cli_help:"comma-separated (e.g. work,ideas)"`
	Pinned  bool     `json:"pinned,omitempty"  desc:"Whether to pin this note to the top"    cli_short:"p"`
}

type NotesUpdateInput struct {
	ID      string   `json:"id"                desc:"Note ID to update"                       cli_short:"i" cli_pos:"1" cli_req:"true"`
	Title   string   `json:"title,omitempty"   desc:"New title"                               cli_short:"t"`
	Content string   `json:"content,omitempty" desc:"New content in Markdown"                 cli_short:"c"`
	Tags    []string `json:"tags,omitempty"    desc:"New tags (replaces existing)"            cli_short:"g" cli_help:"comma-separated"`
}

type NotesGetInput struct {
	ID string `json:"id" desc:"Note ID" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type NotesDeleteInput struct {
	ID string `json:"id" desc:"Note ID to delete" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type NotesSearchInput struct {
	Query string `json:"query" desc:"Search query to match against note titles and content" cli_short:"q" cli_pos:"1" cli_req:"true"`
}

type NotesPinInput struct {
	ID     string `json:"id"     desc:"Note ID"                  cli_short:"i" cli_pos:"1" cli_req:"true"`
	Pinned bool   `json:"pinned" desc:"True to pin, false to unpin" cli_short:"p"`
}

const (
	NotesRead  ToolGroup = "notes_read"
	NotesWrite ToolGroup = "notes_write"
)

func GetNotesGroups() map[ToolGroup]string {
	return map[ToolGroup]string{
		NotesRead:  "Read and discover notes: list, view, and search note content",
		NotesWrite: "Create, update, delete, and organize notes (pin/unpin)",
	}
}

func GetNotesEntries() []ToolEntry {
	return []ToolEntry{
		{
			NewToolDef(
				"notes",
				"notes_create",
				"Create one or more notes with a title and optional Markdown content. Always use this tool even for single-note creation.",
				SchemaFromStruct(NotesCreateInput{}),
				NotesWrite,
			),
			NotesCreateInput{},
		},
		{
			NewToolDef(
				"notes",
				"notes_update",
				"Update the title, content, or tags of an existing note",
				SchemaFromStruct(NotesUpdateInput{}),
				NotesWrite,
			),
			NotesUpdateInput{},
		},
		{
			NewToolDef(
				"notes",
				"notes_get",
				"Get the full content of a specific note by ID",
				SchemaFromStruct(NotesGetInput{}),
				NotesRead,
			),
			NotesGetInput{},
		},
		{
			NewToolDef(
				"notes",
				"notes_delete",
				"Delete a note by ID",
				SchemaFromStruct(NotesDeleteInput{}),
				NotesWrite,
			),
			NotesDeleteInput{},
		},
		{
			NewToolDef(
				"notes",
				"notes_list",
				"List all notes, pinned notes appear first",
				SchemaFromStruct(struct{}{}),
				NotesRead,
			),
			struct{}{},
		},
		{
			NewToolDef(
				"notes",
				"notes_search",
				"Search notes by title and content using full-text search",
				SchemaFromStruct(NotesSearchInput{}),
				NotesRead,
			),
			NotesSearchInput{},
		},
		{
			NewToolDef(
				"notes",
				"notes_pin",
				"Pin or unpin a note",
				SchemaFromStruct(NotesPinInput{}),
				NotesWrite,
			),
			NotesPinInput{},
		},
	}
}

func GetNotesTools() []ToolDef {
	entries := GetNotesEntries()
	defs := make([]ToolDef, len(entries))
	for i, e := range entries {
		defs[i] = e.Def
	}
	return defs
}
