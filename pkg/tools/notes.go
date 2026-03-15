package tools

type NotesCreateInput struct {
	Notes []NotesCreateItem `json:"notes" desc:"One or more notes to create"`
}

type NotesCreateItem struct {
	Title   string   `json:"title"             desc:"Note title"`
	Content string   `json:"content,omitempty" desc:"Note body content in Markdown"`
	Tags    []string `json:"tags,omitempty"    desc:"Tags to categorize the note"`
	Pinned  bool     `json:"pinned,omitempty"  desc:"Whether to pin this note to the top"`
}

type NotesUpdateInput struct {
	ID      string   `json:"id"               desc:"Note ID to update"`
	Title   string   `json:"title,omitempty"   desc:"New title"`
	Content string   `json:"content,omitempty" desc:"New content in Markdown"`
	Tags    []string `json:"tags,omitempty"    desc:"New tags (replaces existing)"`
}

type NotesGetInput struct {
	ID string `json:"id" desc:"Note ID"`
}

type NotesDeleteInput struct {
	ID string `json:"id" desc:"Note ID to delete"`
}

type NotesSearchInput struct {
	Query string `json:"query" desc:"Search query to match against note titles and content"`
}

type NotesPinInput struct {
	ID     string `json:"id"     desc:"Note ID"`
	Pinned bool   `json:"pinned" desc:"True to pin, false to unpin"`
}

func GetNotesTools() []ToolDef {
	return []ToolDef{
		NewToolDef(
			"notes",
			"notes_create",
			"Create one or more notes with a title and optional Markdown content. Always use this tool even for single-note creation.",
			SchemaFromStruct(NotesCreateInput{}),
		),
		NewToolDef(
			"notes",
			"notes_update",
			"Update the title, content, or tags of an existing note",
			SchemaFromStruct(NotesUpdateInput{}),
		),
		NewToolDef(
			"notes",
			"notes_get",
			"Get the full content of a specific note by ID",
			SchemaFromStruct(NotesGetInput{}),
		),
		NewToolDef(
			"notes",
			"notes_delete",
			"Delete a note by ID",
			SchemaFromStruct(NotesDeleteInput{}),
		),
		NewToolDef(
			"notes",
			"notes_list",
			"List all notes, pinned notes appear first",
			SchemaFromStruct(struct{}{}),
		),
		NewToolDef(
			"notes",
			"notes_search",
			"Search notes by title and content using full-text search",
			SchemaFromStruct(NotesSearchInput{}),
		),
		NewToolDef(
			"notes",
			"notes_pin",
			"Pin or unpin a note",
			SchemaFromStruct(NotesPinInput{}),
		),
	}
}
