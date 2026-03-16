package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
)

// Config holds gmail-specific configuration
type Config struct {
	credentials Credentials
}

// GmailConnector implements EmailConnector interface
type GmailConnector struct {
	ctx    context.Context
	cancel context.CancelFunc

	*connectors.BaseConnector

	service *gmail.Service

	cfg *Config
}

// NewConnector creates a new Gmail connector
type Credentials struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
}

func New(ctx context.Context, cfg Config) (*GmailConnector, error) {
	if cfg.credentials.ClientID == "" || cfg.credentials.ClientSecret == "" || cfg.credentials.RefreshToken == "" {
		return nil, fmt.Errorf("gmail: missing required credentials")
	}

	connCtx, cancel := context.WithCancel(ctx)

	return &GmailConnector{
		BaseConnector: connectors.NewBaseConnector(connectors.BaseConnectorInfo{
			ID:           "gmail",
			Name:         "gmail",
			Description:  "Gmail email connector",
			Instructions: "Provides Gmail email operations",
			Enabled:      true,
			Type:         "email",
		}),
		ctx:    connCtx,
		cancel: cancel,
		cfg:    &cfg,
	}, nil
}

// init registers the Gmail connector factory
// Kernel will call this factory to create instances
func init() {
	connectors.Register("gmail", func(ctx context.Context, cfg config.Connector) (connectors.Connector, error) {
		v, err := vault.Load()
		if err != nil {
			return nil, fmt.Errorf("gmail: vault: %w", err)
		}

		var gmailCfg Config

		gmailCfg.credentials.ClientID, err = v.GetSecret(ctx, cfg.VaultPaths["client_id"].Path)
		if err != nil {
			return nil, fmt.Errorf("gmail: get client_id: %w", err)
		}
		gmailCfg.credentials.ClientSecret, err = v.GetSecret(ctx, cfg.VaultPaths["client_secret"].Path)
		if err != nil {
			return nil, fmt.Errorf("gmail: get client_secret: %w", err)
		}
		gmailCfg.credentials.RefreshToken, err = v.GetSecret(ctx, cfg.VaultPaths["refresh_token"].Path)
		if err != nil {
			return nil, fmt.Errorf("gmail: get refresh_token: %w", err)
		}

		conn, err := New(ctx, gmailCfg)
		if err != nil {
			return nil, err
		}

		// override base connector info from config
		conn.BaseConnector = connectors.NewBaseConnectorFromConfig(cfg)

		return conn, nil
	})
}

// ====================
// Connector Interface
// ====================
func (g *GmailConnector) Kind() string { return "email" }

func (g *GmailConnector) Connect() error {
	oauthConfig := &oauth2.Config{
		ClientID:     g.cfg.credentials.ClientID,
		ClientSecret: g.cfg.credentials.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{gmail.GmailModifyScope, gmail.GmailSendScope},
	}
	token := &oauth2.Token{
		RefreshToken: g.cfg.credentials.RefreshToken,
		TokenType:    "Bearer",
	}
	service, err := gmail.NewService(g.ctx, option.WithHTTPClient(oauthConfig.Client(g.ctx, token)))
	if err != nil {
		return fmt.Errorf("create gmail service: %w", err)
	}
	g.service = service
	return nil
}

func (g *GmailConnector) Disconnect() error {
	g.service = nil
	g.cancel()
	return nil
}

func (g *GmailConnector) Start() error {
	// Gmail doesn't need a start process
	return nil
}

func (g *GmailConnector) Stop() error {
	// Gmail doesn't need a stop process
	return nil
}

func (g *GmailConnector) HealthCheck() error {
	if g.service == nil {
		return fmt.Errorf("gmail service not connected")
	}

	// Try to get user profile
	_, err := g.service.Users.GetProfile("me").Do()
	return err
}

// ====================
// EmailConnector Interface
// ====================

func (g *GmailConnector) SendEmail(ctx context.Context, req *connectors.SendEmailRequest) error {
	if g.service == nil {
		return fmt.Errorf("gmail service not connected")
	}

	// Build email message
	var message strings.Builder

	fmt.Fprintf(&message, "To: %s\r\n", strings.Join(req.To, ","))

	if len(req.Cc) > 0 {
		fmt.Fprintf(&message, "Cc: %s\r\n", strings.Join(req.Cc, ","))
	}

	if len(req.Bcc) > 0 {
		fmt.Fprintf(&message, "Bcc: %s\r\n", strings.Join(req.Bcc, ","))
	}

	fmt.Fprintf(&message, "Subject: %s\r\n", req.Subject)
	message.WriteString("MIME-Version: 1.0\r\n")

	if req.BodyHTML != "" {
		message.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
		message.WriteString(req.BodyHTML)
	} else {
		message.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		message.WriteString(req.Body)
	}

	// Encode message
	encoded := base64.URLEncoding.EncodeToString([]byte(message.String()))

	gmailMessage := &gmail.Message{
		Raw: encoded,
	}

	// Send
	_, err := g.service.Users.Messages.Send("me", gmailMessage).Do()
	return err
}

func (g *GmailConnector) DraftEmail(ctx context.Context, req *connectors.SendEmailRequest) error {
	return nil
}

func (g *GmailConnector) GetEmail(ctx context.Context, id string) (*connectors.Email, error) {
	if g.service == nil {
		return nil, fmt.Errorf("gmail service not connected")
	}

	msg, err := g.service.Users.Messages.Get("me", id).Format("full").Do()
	if err != nil {
		return nil, err
	}

	return g.convertMessage(msg), nil
}

func (g *GmailConnector) SearchEmails(ctx context.Context, req *connectors.SearchEmailsRequest) ([]*connectors.Email, error) {
	if g.service == nil {
		return nil, fmt.Errorf("gmail service not connected")
	}

	// Build query
	query := req.Query
	if req.From != "" {
		query += fmt.Sprintf(" from:%s", req.From)
	}
	if req.To != "" {
		query += fmt.Sprintf(" to:%s", req.To)
	}
	if req.Subject != "" {
		query += fmt.Sprintf(" subject:%s", req.Subject)
	}
	if req.After != nil {
		query += fmt.Sprintf(" after:%s", req.After.Format("2006/01/02"))
	}
	if req.Before != nil {
		query += fmt.Sprintf(" before:%s", req.Before.Format("2006/01/02"))
	}
	if req.HasAttachment {
		query += " has:attachment"
	}
	if req.IsUnread {
		query += " is:unread"
	}

	maxResults := int64(req.MaxResults)
	if maxResults == 0 {
		maxResults = 10
	}

	listCall := g.service.Users.Messages.List("me").Q(query).MaxResults(maxResults)

	resp, err := listCall.Do()
	if err != nil {
		return nil, err
	}

	var emails []*connectors.Email
	for _, msg := range resp.Messages {
		fullMsg, err := g.service.Users.Messages.Get("me", msg.Id).Format("full").Do()
		if err != nil {
			continue
		}
		emails = append(emails, g.convertMessage(fullMsg))
	}

	return emails, nil
}

func (g *GmailConnector) DeleteEmail(ctx context.Context, id string) error {
	if g.service == nil {
		return fmt.Errorf("gmail service not connected")
	}

	return g.service.Users.Messages.Delete("me", id).Do()
}

func (g *GmailConnector) MarkAsRead(ctx context.Context, id string) error {
	if g.service == nil {
		return fmt.Errorf("gmail service not connected")
	}

	req := &gmail.ModifyMessageRequest{
		RemoveLabelIds: []string{"UNREAD"},
	}
	_, err := g.service.Users.Messages.Modify("me", id, req).Do()
	return err
}

func (g *GmailConnector) MarkAsUnread(ctx context.Context, id string) error {
	if g.service == nil {
		return fmt.Errorf("gmail service not connected")
	}

	req := &gmail.ModifyMessageRequest{
		AddLabelIds: []string{"UNREAD"},
	}
	_, err := g.service.Users.Messages.Modify("me", id, req).Do()
	return err
}

// ====================
// Helper Methods
// ====================

func (g *GmailConnector) convertMessage(msg *gmail.Message) *connectors.Email {
	email := &connectors.Email{
		ID:       msg.Id,
		ThreadID: msg.ThreadId,
		Labels:   msg.LabelIds,
		Raw:      msg,
	}

	// Parse headers
	for _, header := range msg.Payload.Headers {
		switch header.Name {
		case "From":
			email.From = parseEmailAddress(header.Value)
		case "To":
			email.To = parseEmailAddresses(header.Value)
		case "Cc":
			email.Cc = parseEmailAddresses(header.Value)
		case "Subject":
			email.Subject = header.Value
		case "Date":
			if t, err := time.Parse(time.RFC1123Z, header.Value); err == nil {
				email.ReceivedAt = t
			}
		}
	}

	// Get body
	body, err := getBody(msg.Payload)
	if err != nil {
		fmt.Printf("error getting body: %v", err)
		return nil
	}
	email.Body = body

	// Check if read
	email.IsRead = true
	email.IsRead = !slices.Contains(msg.LabelIds, "UNREAD")

	return email
}

func parseEmailAddress(s string) connectors.EmailAddress {
	// Simple parser - can be improved
	parts := strings.Split(s, "<")
	if len(parts) == 2 {
		name := strings.TrimSpace(parts[0])
		email := strings.TrimSuffix(strings.TrimSpace(parts[1]), ">")
		return connectors.EmailAddress{Email: email, Name: name}
	}
	return connectors.EmailAddress{Email: strings.TrimSpace(s)}
}

func parseEmailAddresses(s string) []connectors.EmailAddress {
	addresses := strings.Split(s, ",")
	result := make([]connectors.EmailAddress, 0, len(addresses))
	for _, addr := range addresses {
		result = append(result, parseEmailAddress(addr))
	}
	return result
}

func getBody(payload *gmail.MessagePart) (string, error) {
	if payload.Body != nil && payload.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}

	for _, part := range payload.Parts {
		if part.MimeType == "text/plain" || part.MimeType == "text/html" {
			if part.Body != nil && part.Body.Data != "" {
				decoded, err := base64.URLEncoding.DecodeString(part.Body.Data)
				if err != nil {
					return "", err
				}
				return string(decoded), nil
			}
		}
	}

	return "", nil
}
