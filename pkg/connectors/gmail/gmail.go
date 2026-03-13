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

	"github.com/go-viper/mapstructure/v2"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// init registers the Gmail connector factory
// Kernel will call this factory to create instances
func init() {
	connectors.Register("gmail", NewConnector)
}

// Config holds Gmail-specific configuration
type Config struct {
	config.ConnectorConfig
	// OAuth
	OAuthConfig *oauth2.Config `yaml:"-"` // Built from credentials
}

// GmailConnector implements EmailConnector interface
type GmailConnector struct {
	ctx      context.Context
	cancel   context.CancelFunc
	config   *Config
	vault    vault.Vault
	eventBus chan<- core.Event
	service  *gmail.Service
}

// NewConnector creates a new Gmail connector
func NewConnector(
	ctx context.Context,
	cfg config.ConnectorConfig,
	v vault.Vault,
	eventBus chan<- core.Event,
) (connectors.Connector, error) {
	gmailCfg := &Config{
		ConnectorConfig: cfg,
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &gmailCfg,
		WeaklyTypedInput: true,
		TagName:          "mapstructure",
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("create decoder: %w", err)
	}
	if err := decoder.Decode(cfg.RawConfig); err != nil {
		return nil, fmt.Errorf("decode gmail config: %w", err)
	}

	connCtx, cancel := context.WithCancel(ctx) //nolint:gosec
	return &GmailConnector{
		ctx:      connCtx,
		cancel:   cancel,
		config:   gmailCfg,
		vault:    v,
		eventBus: eventBus,
	}, nil
}

// ====================
// Connector Interface
// ====================

func (g *GmailConnector) Name() string                   { return g.config.Name }
func (g *GmailConnector) ID() string                     { return g.config.ID }
func (g *GmailConnector) Type() connectors.ConnectorType { return connectors.ConnectorTypeService }
func (g *GmailConnector) Kind() string                   { return "email" }

func (g *GmailConnector) Connect() error {
	// Get credentials from vault
	clientID, err := g.vault.GetSecret(g.ctx, g.config.VaultPaths["client_id"].Path)
	if err != nil {
		return fmt.Errorf("get client_id: %w", err)
	}

	clientSecret, err := g.vault.GetSecret(g.ctx, g.config.VaultPaths["client_secret"].Path)
	if err != nil {
		return fmt.Errorf("get client_secret: %w", err)
	}

	refreshToken, err := g.vault.GetSecret(g.ctx, g.config.VaultPaths["refresh_token"].Path)
	if err != nil {
		return fmt.Errorf("get refresh_token: %w", err)
	}

	// Configure OAuth2
	oauthConfig := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		Scopes: []string{
			gmail.GmailModifyScope,
			gmail.GmailSendScope,
		},
	}

	token := &oauth2.Token{
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
	}

	httpClient := oauthConfig.Client(g.ctx, token)

	// Create Gmail service
	service, err := gmail.NewService(g.ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return fmt.Errorf("create gmail service: %w", err)
	}

	g.service = service
	return nil
}

func (g *GmailConnector) Disconnect() error {
	g.service = nil
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
