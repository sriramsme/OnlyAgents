package email

import (
	"context"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

const (
	version = "1.0.0"
)

func init() {
	skills.Register("email", NewEmailSkill)
}

// EmailSkill provides email management capabilities
type EmailSkill struct {
	ctx      context.Context
	cancel   context.CancelFunc
	eventBus chan<- core.Event

	*skills.BaseSkill

	// Connectors injected by kernel
	// Cast to connectors.EmailConnector when using
	emailConns map[string]connectors.EmailConnector
}

// NewEmailSkill creates a new email skill
func NewEmailSkill(ctx context.Context, eventBus chan<- core.Event) (skills.Skill, error) {
	base := skills.NewBaseSkill(
		"email",
		"Manage emails - send, search, read, and draft emails using AI",
		version,
		skills.SkillTypeNative,
	)

	skillCtx, cancel := context.WithCancel(ctx)
	return &EmailSkill{
		BaseSkill:  base,
		emailConns: make(map[string]connectors.EmailConnector),
		ctx:        skillCtx,
		cancel:     cancel,
		eventBus:   eventBus,
	}, nil
}

// Initialize sets up the email skill with injected connectors
func (s *EmailSkill) Initialize(deps skills.SkillDeps) error {
	s.SetOutbox(deps.Outbox)

	// Extract email connectors from deps.Connectors
	// Kernel has already filtered and injected the right ones
	for name, conn := range deps.Connectors {
		if emailConn, ok := conn.(connectors.EmailConnector); ok {
			s.emailConns[name] = emailConn
		}
	}

	if len(s.emailConns) == 0 {
		logger.Log.Error("email skill requires at least one email connector")
	}

	return nil
}

// Shutdown cleans up resources
func (s *EmailSkill) Shutdown() error {
	return nil
}

// RequiredCapabilities declares that this skill needs email connectors
func (s *EmailSkill) RequiredCapabilities() []core.Capability {
	return []core.Capability{core.CapabilityEmail}
}

// Tools returns the LLM function calling tools for email
func (s *EmailSkill) Tools() []tools.ToolDef {
	return []tools.ToolDef{
		tools.NewToolDef(
			"email_send",
			"Send an email to one or more recipients",
			tools.BuildParams(
				map[string]tools.Property{
					"to":      tools.ArrayProp("Email addresses to send to", tools.StringProp("")),
					"subject": tools.StringProp("Email subject line"),
					"body":    tools.StringProp("Email body content"),
					"cc":      tools.ArrayProp("CC email addresses (optional)", tools.StringProp("")),
				},
				[]string{"to", "subject", "body"},
			),
		),
		tools.NewToolDef(
			"email_search",
			"Search for emails matching criteria",
			tools.BuildParams(
				map[string]tools.Property{
					"query":       tools.StringProp("Search query (keywords, phrases)"),
					"from":        tools.StringProp("Filter by sender email (optional)"),
					"max_results": tools.IntProp("Maximum number of results (default: 10)"),
					"is_unread":   tools.BoolProp("Only return unread emails (optional)"),
				},
				[]string{"query"},
			),
		),
		tools.NewToolDef(
			"email_get",
			"Get full details of a specific email by ID",
			tools.BuildParams(
				map[string]tools.Property{
					"email_id": tools.StringProp("The email ID"),
				},
				[]string{"email_id"},
			),
		),
		tools.NewToolDef(
			"email_draft",
			"Use AI to draft an email based on context and tone. This will request a sub-agent to generate the draft.",
			tools.BuildParams(
				map[string]tools.Property{
					"context": tools.StringProp("Context for the email (what to say, who it's to, purpose)"),
					"tone":    tools.EnumProp("Tone of the email", []string{"professional", "casual", "friendly", "formal"}),
				},
				[]string{"context"},
			),
		),
		tools.NewToolDef(
			"email_mark_read",
			"Mark an email as read",
			tools.BuildParams(
				map[string]tools.Property{
					"email_id": tools.StringProp("The email ID"),
				},
				[]string{"email_id"},
			),
		),
		tools.NewToolDef(
			"email_delete",
			"Delete an email",
			tools.BuildParams(
				map[string]tools.Property{
					"email_id": tools.StringProp("The email ID to delete"),
				},
				[]string{"email_id"},
			),
		),
	}
}

// Execute runs a tool
func (s *EmailSkill) Execute(ctx context.Context, toolName string, params map[string]any) (any, error) {
	switch toolName {
	case "email_send":
		return s.sendEmail(ctx, params)
	case "email_search":
		return s.searchEmails(ctx, params)
	case "email_get":
		return s.getEmail(ctx, params)
	case "email_draft":
		return s.draftEmail(ctx, params)
	case "email_mark_read":
		return s.markAsRead(ctx, params)
	case "email_delete":
		return s.deleteEmail(ctx, params)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// ====================
// Tool Implementations
// ====================

func (s *EmailSkill) sendEmail(ctx context.Context, params map[string]any) (any, error) {
	// Use first available email connector (kernel ensures at least one exists)
	var emailConn connectors.EmailConnector
	for _, conn := range s.emailConns {
		emailConn = conn
		break
	}

	// Parse recipients
	toInterfaces := params["to"].([]interface{})
	to := make([]string, len(toInterfaces))
	for i, v := range toInterfaces {
		to[i] = v.(string)
	}

	req := &connectors.SendEmailRequest{
		To:      to,
		Subject: params["subject"].(string),
		Body:    params["body"].(string),
	}

	// Optional CC
	if ccVal, ok := params["cc"]; ok {
		ccInterfaces := ccVal.([]interface{})
		cc := make([]string, len(ccInterfaces))
		for i, v := range ccInterfaces {
			cc[i] = v.(string)
		}
		req.Cc = cc
	}

	err := emailConn.SendEmail(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("send email: %w", err)
	}

	return map[string]any{
		"status":  "sent",
		"to":      to,
		"subject": req.Subject,
	}, nil
}

func (s *EmailSkill) searchEmails(ctx context.Context, params map[string]any) (any, error) {
	var emailConn connectors.EmailConnector
	for _, conn := range s.emailConns {
		emailConn = conn
		break
	}

	req := &connectors.SearchEmailsRequest{
		Query:      params["query"].(string),
		MaxResults: 10,
	}

	if val, ok := params["from"]; ok {
		req.From = val.(string)
	}
	if val, ok := params["max_results"]; ok {
		req.MaxResults = int(val.(float64))
	}
	if val, ok := params["is_unread"]; ok {
		req.IsUnread = val.(bool)
	}

	emails, err := emailConn.SearchEmails(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("search emails: %w", err)
	}

	// Return simplified email list
	results := make([]map[string]any, len(emails))
	for i, email := range emails {
		results[i] = map[string]any{
			"id":       email.ID,
			"from":     email.From.Email,
			"subject":  email.Subject,
			"snippet":  truncate(email.Body, 150),
			"is_read":  email.IsRead,
			"received": email.ReceivedAt.Format(time.RFC3339),
		}
	}

	return map[string]any{
		"count":  len(results),
		"emails": results,
	}, nil
}

func (s *EmailSkill) getEmail(ctx context.Context, params map[string]any) (any, error) {
	var emailConn connectors.EmailConnector
	for _, conn := range s.emailConns {
		emailConn = conn
		break
	}

	emailID := params["email_id"].(string)

	email, err := emailConn.GetEmail(ctx, emailID)
	if err != nil {
		return nil, fmt.Errorf("get email: %w", err)
	}

	return map[string]any{
		"id":       email.ID,
		"from":     email.From.Email,
		"to":       getEmailStrings(email.To),
		"subject":  email.Subject,
		"body":     email.Body,
		"is_read":  email.IsRead,
		"received": email.ReceivedAt.Format(time.RFC3339),
	}, nil
}

func (s *EmailSkill) draftEmail(ctx context.Context, params map[string]any) (any, error) {
	// Fire AgentRequest event to kernel
	// Kernel will route to an agent that can draft text
	contextStr := params["context"].(string)
	tone := "professional"
	if val, ok := params["tone"]; ok {
		tone = val.(string)
	}

	// Fire event (non-blocking)
	// Note: This is async - we return immediately.
	// For sync behavior, skill would need a reply channel pattern.
	s.RequestSubAgent(ctx, "", fmt.Sprintf("Draft a %s email: %s", tone, contextStr), map[string]any{
		"task_type": "email_draft",
		"tone":      tone,
	})

	return map[string]any{
		"status":  "draft_requested",
		"message": "AI agent is drafting the email. This is an async operation.",
	}, nil
}

func (s *EmailSkill) markAsRead(ctx context.Context, params map[string]any) (any, error) {
	var emailConn connectors.EmailConnector
	for _, conn := range s.emailConns {
		emailConn = conn
		break
	}

	emailID := params["email_id"].(string)

	err := emailConn.MarkAsRead(ctx, emailID)
	if err != nil {
		return nil, fmt.Errorf("mark as read: %w", err)
	}

	return map[string]any{
		"status":   "marked_read",
		"email_id": emailID,
	}, nil
}

func (s *EmailSkill) deleteEmail(ctx context.Context, params map[string]any) (any, error) {
	var emailConn connectors.EmailConnector
	for _, conn := range s.emailConns {
		emailConn = conn
		break
	}

	emailID := params["email_id"].(string)

	err := emailConn.DeleteEmail(ctx, emailID)
	if err != nil {
		return nil, fmt.Errorf("delete email: %w", err)
	}

	return map[string]any{
		"status":   "deleted",
		"email_id": emailID,
	}, nil
}

// ====================
// Helper Methods
// ====================

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func getEmailStrings(addresses []connectors.EmailAddress) []string {
	result := make([]string, len(addresses))
	for i, addr := range addresses {
		result[i] = addr.Email
	}
	return result
}
