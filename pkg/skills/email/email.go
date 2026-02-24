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
	return tools.GetEmailTools()
}

// Execute runs a tool
func (s *EmailSkill) Execute(ctx context.Context, toolName string, args []byte) (any, error) {
	switch toolName {
	case "email_send":
		return s.sendEmail(ctx, args)
	case "email_search":
		return s.searchEmails(ctx, args)
	case "email_get":
		return s.getEmail(ctx, args)
	case "email_draft":
		return s.draftEmail(ctx, args)
	case "email_mark_read":
		return s.markAsRead(ctx, args)
	case "email_delete":
		return s.deleteEmail(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// ====================
// Tool Implementations
// ====================

func (s *EmailSkill) sendEmail(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.EmailSendInput](args)
	if err != nil {
		return nil, err
	}
	emailConn, err := s.getConnector()
	if err != nil {
		return nil, err
	}

	req := &connectors.SendEmailRequest{
		To:      input.To,
		Subject: input.Subject,
		Body:    input.Body,
		Cc:      input.CC,
	}

	err = emailConn.SendEmail(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("send email: %w", err)
	}

	return map[string]any{
		"status":  "sent",
		"to":      req.To,
		"subject": req.Subject,
	}, nil
}

func (s *EmailSkill) searchEmails(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.EmailSearchInput](args)
	if err != nil {
		return nil, err
	}
	emailConn, err := s.getConnector()
	if err != nil {
		return nil, err
	}
	req := &connectors.SearchEmailsRequest{
		Query:      input.Query,
		From:       input.From,
		MaxResults: input.MaxResults,
		IsUnread:   input.IsUnread,
	}
	if req.MaxResults == 0 {
		req.MaxResults = 10
	}
	emails, err := emailConn.SearchEmails(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("search emails: %w", err)
	}
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

func (s *EmailSkill) getEmail(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.EmailGetInput](args)
	if err != nil {
		return nil, err
	}
	emailConn, err := s.getConnector()
	if err != nil {
		return nil, err
	}
	email, err := emailConn.GetEmail(ctx, input.EmailID)
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

func (s *EmailSkill) draftEmail(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.EmailDraftInput](args)
	if err != nil {
		return nil, err
	}
	emailConn, err := s.getConnector()
	if err != nil {
		return nil, err
	}
	// draftEmail doesn't actually send — it returns a draft for the LLM to confirm
	_ = emailConn
	return map[string]any{
		"status":  "drafted",
		"context": input.Context,
		"tone":    input.Tone,
	}, nil
}

func (s *EmailSkill) markAsRead(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.EmailMarkReadInput](args)
	if err != nil {
		return nil, err
	}
	emailConn, err := s.getConnector()
	if err != nil {
		return nil, err
	}
	if err := emailConn.MarkAsRead(ctx, input.EmailID); err != nil {
		return nil, fmt.Errorf("mark as read: %w", err)
	}
	return map[string]any{
		"status":   "marked_read",
		"email_id": input.EmailID,
	}, nil
}

func (s *EmailSkill) deleteEmail(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.EmailDeleteInput](args)
	if err != nil {
		return nil, err
	}
	emailConn, err := s.getConnector()
	if err != nil {
		return nil, err
	}
	if err := emailConn.DeleteEmail(ctx, input.EmailID); err != nil {
		return nil, fmt.Errorf("delete email: %w", err)
	}
	return map[string]any{
		"status":   "deleted",
		"email_id": input.EmailID,
	}, nil
}

// ====================
// Helper Methods
// ====================

func (s *EmailSkill) getConnector() (connectors.EmailConnector, error) {
	for _, conn := range s.emailConns {
		return conn, nil
	}
	return nil, fmt.Errorf("no email connector available")
}

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
