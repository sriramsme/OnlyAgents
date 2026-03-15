package tools

// ====================
// Input Types
// ====================

type EmailSendInput struct {
	To      []string `json:"to" desc:"Email addresses to send to"`
	Subject string   `json:"subject" desc:"Email subject line"`
	Body    string   `json:"body" desc:"Email body content"`
	CC      []string `json:"cc,omitempty" desc:"CC email addresses"`
}

type EmailSearchInput struct {
	Query      string `json:"query" desc:"Search query (keywords, phrases)"`
	From       string `json:"from,omitempty" desc:"Filter by sender email"`
	MaxResults int    `json:"max_results,omitempty" desc:"Maximum number of results (default: 10)"`
	IsUnread   bool   `json:"is_unread,omitempty" desc:"Only return unread emails"`
}

type EmailGetInput struct {
	EmailID string `json:"email_id" desc:"The email ID"`
}

type EmailDraftInput struct {
	Context string `json:"context" desc:"Context for the email (what to say, who it's to, purpose)"`
	Tone    string `json:"tone,omitempty" desc:"Tone of the email" enum:"professional,casual,friendly,formal"`
}

type EmailMarkReadInput struct {
	EmailID string `json:"email_id" desc:"The email ID"`
}

type EmailDeleteInput struct {
	EmailID string `json:"email_id" desc:"The email ID to delete"`
}

func GetEmailTools() []ToolDef {
	return []ToolDef{
		NewToolDef(
			"email",
			"email_send",
			"Send an email to one or more recipients",
			SchemaFromStruct(EmailSendInput{}),
		),
		NewToolDef(
			"email",
			"email_search",
			"Search for emails matching criteria",
			SchemaFromStruct(EmailSearchInput{}),
		),
		NewToolDef(
			"email",
			"email_get",
			"Get full details of a specific email by ID",
			SchemaFromStruct(EmailGetInput{}),
		),
		NewToolDef(
			"email",
			"email_draft",
			"Use AI to draft an email based on context and tone",
			SchemaFromStruct(EmailDraftInput{}),
		),
		NewToolDef(
			"email",
			"email_mark_read",
			"Mark an email as read",
			SchemaFromStruct(EmailMarkReadInput{}),
		),
		NewToolDef(
			"email",
			"email_delete",
			"Delete an email",
			SchemaFromStruct(EmailDeleteInput{}),
		),
	}
}
