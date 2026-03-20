package email

import (
	"context"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// EmailSkill provides email management capabilities
type EmailSkill struct {
	ctx    context.Context
	cancel context.CancelFunc
	*skills.BaseSkill

	// Connectors injected by kernel
	// Cast to connectors.EmailConnector when using
	conn connectors.EmailConnector
}

// NewEmailSkill creates a new email skill
// external path — defaults baked in
func New(ctx context.Context, conn connectors.EmailConnector) (*EmailSkill, error) {
	if conn == nil {
		return nil, fmt.Errorf("email: connector required")
	}

	skillCtx, cancel := context.WithCancel(ctx)

	return &EmailSkill{
		BaseSkill: skills.NewBaseSkill(skills.BaseSkillInfo{
			Name:        "email",
			Description: "Send, search, and manage emails",
			Version:     "1.0.0",
			Enabled:     true,
			AccessLevel: "read",
			Tools:       tools.GetEmailTools(),
			Groups:      tools.GetEmailGroups(),
		}, skills.SkillTypeNative),
		conn:   conn,
		ctx:    skillCtx,
		cancel: cancel,
	}, nil
}

// internal path — config drives everything, never touches New()
func init() {
	skills.Register("email", func(
		ctx context.Context,
		cfg skills.Config,
		conn connectors.Connector,
	) (skills.Skill, error) {
		emailConn, ok := conn.(connectors.EmailConnector)
		if !ok {
			return nil, fmt.Errorf("email: connector is not an EmailConnector")
		}

		skillCtx, cancel := context.WithCancel(ctx)

		return &EmailSkill{
			BaseSkill: skills.NewBaseSkillFromConfig(
				cfg,
				skills.SkillTypeNative,
				tools.GetEmailTools(),
				tools.GetEmailGroups(),
			),
			conn:   emailConn,
			ctx:    skillCtx,
			cancel: cancel,
		}, nil
	})
}

// Initialize sets up the email skill with injected connectors
func (s *EmailSkill) Initialize() error {
	return nil
}

// Shutdown cleans up resources
func (s *EmailSkill) Shutdown() error {
	s.cancel()
	return nil
}

// Execute runs a tool
func (s *EmailSkill) Execute(ctx context.Context, toolName string, args []byte) tools.ToolExecution {
	var result any
	var err error

	switch toolName {
	case "email_send":
		result, err = s.sendEmail(ctx, args)
	case "email_search":
		result, err = s.searchEmails(ctx, args)
	case "email_get":
		result, err = s.getEmail(ctx, args)
	case "email_draft":
		result, err = s.draftEmail(ctx, args)
	case "email_mark_read":
		result, err = s.markAsRead(ctx, args)
	case "email_delete":
		result, err = s.deleteEmail(ctx, args)
	default:
		return tools.ExecErr(fmt.Errorf("unknown tool: %s", toolName))
	}

	if err != nil {
		return tools.ExecErr(err)
	}
	return tools.ExecOK(result)
}

// ====================
// Tool Implementations
// ====================

func (s *EmailSkill) sendEmail(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.EmailSendInput](args)
	if err != nil {
		return nil, err
	}

	req := &connectors.SendEmailRequest{
		To:      input.To,
		Subject: input.Subject,
		Body:    input.Body,
		Cc:      input.CC,
	}

	err = s.conn.SendEmail(ctx, req)
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

	req := &connectors.SearchEmailsRequest{
		Query:      input.Query,
		From:       input.From,
		MaxResults: input.MaxResults,
		IsUnread:   input.IsUnread,
	}
	if req.MaxResults == 0 {
		req.MaxResults = 10
	}
	emails, err := s.conn.SearchEmails(ctx, req)
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
	email, err := s.conn.GetEmail(ctx, input.EmailID)
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

	if err := s.conn.MarkAsRead(ctx, input.EmailID); err != nil {
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

	if err := s.conn.DeleteEmail(ctx, input.EmailID); err != nil {
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
