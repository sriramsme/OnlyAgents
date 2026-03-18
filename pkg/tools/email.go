package tools

// ====================
// Input Types
// ====================

type EmailSendInput struct {
	To      []string `json:"to"              desc:"Email addresses to send to"                  cli_short:"t" cli_req:"true" cli_help:"comma-separated (e.g. a@x.com,b@y.com)"`
	Subject string   `json:"subject"         desc:"Email subject line"                          cli_short:"s" cli_req:"true"`
	Body    string   `json:"body"            desc:"Email body content"                          cli_short:"b" cli_req:"true" cli_help:"plain text or markdown"`
	CC      []string `json:"cc,omitempty"    desc:"CC email addresses"                          cli_short:"c" cli_help:"comma-separated"`
}

type EmailSearchInput struct {
	Query      string `json:"query"              desc:"Search query (keywords, phrases)"         cli_short:"q" cli_pos:"1" cli_req:"true"`
	From       string `json:"from,omitempty"     desc:"Filter by sender email"                   cli_short:"f"`
	MaxResults int    `json:"max_results,omitempty" desc:"Maximum number of results (default: 10)" cli_short:"n" cli_help:"e.g. 5, 10, 20"`
	IsUnread   bool   `json:"is_unread,omitempty" desc:"Only return unread emails"               cli_short:"u"`
}

type EmailGetInput struct {
	EmailID string `json:"email_id" desc:"The email ID" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type EmailDraftInput struct {
	Context string `json:"context"         desc:"Context for the email (what to say, who it's to, purpose)" cli_short:"c" cli_pos:"1" cli_req:"true"`
	Tone    string `json:"tone,omitempty"  desc:"Tone of the email"                                          cli_short:"t" cli_help:"professional, casual, friendly, formal" enum:"professional,casual,friendly,formal"`
}

type EmailMarkReadInput struct {
	EmailID string `json:"email_id" desc:"The email ID" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type EmailDeleteInput struct {
	EmailID string `json:"email_id" desc:"The email ID to delete" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

const (
	EmailRead   ToolGroup = "email_read"
	EmailWrite  ToolGroup = "email_write"
	EmailManage ToolGroup = "email_manage"
)

func GetEmailGroups() map[ToolGroup]string {
	return map[ToolGroup]string{
		EmailRead:   "Search and view emails",
		EmailWrite:  "Compose, draft, and send emails",
		EmailManage: "Modify email state such as marking as read or deleting",
	}
}

func GetEmailEntries() []ToolEntry {
	return []ToolEntry{
		{
			NewToolDef(
				"email",
				"email_send",
				"Send an email to one or more recipients",
				SchemaFromStruct(EmailSendInput{}),
				EmailWrite,
			),
			EmailSendInput{},
		},
		{
			NewToolDef(
				"email",
				"email_search",
				"Search for emails matching criteria",
				SchemaFromStruct(EmailSearchInput{}),
				EmailRead,
			),
			EmailSearchInput{},
		},
		{
			NewToolDef(
				"email",
				"email_get",
				"Get full details of a specific email by ID",
				SchemaFromStruct(EmailGetInput{}),
				EmailRead,
			),
			EmailGetInput{},
		},
		{
			NewToolDef(
				"email",
				"email_draft",
				"Use AI to draft an email based on context and tone",
				SchemaFromStruct(EmailDraftInput{}),
				EmailWrite,
			),
			EmailDraftInput{},
		},
		{
			NewToolDef(
				"email",
				"email_mark_read",
				"Mark an email as read",
				SchemaFromStruct(EmailMarkReadInput{}),
				EmailManage,
			),
			EmailMarkReadInput{},
		},
		{
			NewToolDef(
				"email",
				"email_delete",
				"Delete an email",
				SchemaFromStruct(EmailDeleteInput{}),
				EmailManage,
			),
			EmailDeleteInput{},
		},
	}
}

func GetEmailTools() []ToolDef {
	entries := GetEmailEntries()
	defs := make([]ToolDef, len(entries))
	for i, e := range entries {
		defs[i] = e.Def
	}
	return defs
}
