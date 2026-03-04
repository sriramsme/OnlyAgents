package connectors

import (
	"io"
	"time"
)

// ====================
// Email Types
// ====================

type SendEmailRequest struct {
	To          []string          `json:"to"`
	Cc          []string          `json:"cc,omitempty"`
	Bcc         []string          `json:"bcc,omitempty"`
	Subject     string            `json:"subject"`
	Body        string            `json:"body"`
	BodyHTML    string            `json:"body_html,omitempty"`
	Attachments []EmailAttachment `json:"attachments,omitempty"`
	ReplyToID   string            `json:"reply_to_id,omitempty"`
}

type SearchEmailsRequest struct {
	Query         string     `json:"query"`
	MaxResults    int        `json:"max_results"`
	From          string     `json:"from,omitempty"`
	To            string     `json:"to,omitempty"`
	Subject       string     `json:"subject,omitempty"`
	After         *time.Time `json:"after,omitempty"`
	Before        *time.Time `json:"before,omitempty"`
	HasAttachment bool       `json:"has_attachment,omitempty"`
	IsUnread      bool       `json:"is_unread,omitempty"`
}

type Email struct {
	ID          string            `json:"id"`
	ThreadID    string            `json:"thread_id,omitempty"`
	From        EmailAddress      `json:"from"`
	To          []EmailAddress    `json:"to"`
	Cc          []EmailAddress    `json:"cc,omitempty"`
	Bcc         []EmailAddress    `json:"bcc,omitempty"`
	Subject     string            `json:"subject"`
	Body        string            `json:"body"`
	BodyHTML    string            `json:"body_html,omitempty"`
	Attachments []EmailAttachment `json:"attachments,omitempty"`
	ReceivedAt  time.Time         `json:"received_at"`
	IsRead      bool              `json:"is_read"`
	IsStarred   bool              `json:"is_starred"`
	Labels      []string          `json:"labels,omitempty"`
	Raw         interface{}       `json:"raw,omitempty"`
}

type EmailAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type EmailAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Data        []byte `json:"data,omitempty"`
	URL         string `json:"url,omitempty"`
}

// TimeSlot is shared across connectors that deal with availability.
type TimeSlot struct {
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
	Duration time.Duration `json:"duration"`
}

// ====================
// Web Search Types
// ====================
type SearchRequest struct {
	Query      string   `json:"query"`
	MaxResults int      `json:"max_results"`
	Language   string   `json:"language,omitempty"`
	Country    string   `json:"country,omitempty"`
	SafeSearch bool     `json:"safe_search"`
	TimeRange  string   `json:"time_range,omitempty"` // day, week, month, year
	Domains    []string `json:"domains,omitempty"`    // Restrict to specific domains
}

type SearchResponse struct {
	Query      string         `json:"query"`
	Results    []SearchResult `json:"results"`
	TotalCount int            `json:"total_count,omitempty"`
	Provider   string         `json:"provider"` // brave, duckduckgo, perplexity
}

type SearchResult struct {
	Title       string      `json:"title"`
	URL         string      `json:"url"`
	Snippet     string      `json:"snippet"`
	Description string      `json:"description,omitempty"`
	PublishedAt *time.Time  `json:"published_at,omitempty"`
	Source      string      `json:"source,omitempty"`
	Favicon     string      `json:"favicon,omitempty"`
	Raw         interface{} `json:"raw,omitempty"`
}

// ====================
// Web Fetch Types
// ====================

type FetchRequest struct {
	URL      string `json:"url"`
	MaxChars int    `json:"max_chars,omitempty"`
}

type FetchResponse struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Content    string `json:"content"`
	Extractor  string `json:"extractor"` // text, json, raw
	Truncated  bool   `json:"truncated"`
	Length     int    `json:"length"`
}

// ====================
// Storage Types
// ====================

type UploadRequest struct {
	Filename    string            `json:"filename"`
	Content     io.Reader         `json:"-"`
	ContentType string            `json:"content_type,omitempty"`
	FolderID    string            `json:"folder_id,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type FileInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	MimeType    string            `json:"mime_type"`
	Size        int64             `json:"size"`
	FolderID    string            `json:"folder_id,omitempty"`
	WebViewLink string            `json:"web_view_link,omitempty"`
	DownloadURL string            `json:"download_url,omitempty"`
	Shared      bool              `json:"shared"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Raw         interface{}       `json:"raw,omitempty"`
}

type ListFilesRequest struct {
	FolderID   string `json:"folder_id,omitempty"`
	Query      string `json:"query,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

type ShareRequest struct {
	Type           string   `json:"type"` // anyone, user, domain
	Role           string   `json:"role"` // reader, writer, commenter
	EmailAddresses []string `json:"email_addresses,omitempty"`
	Domain         string   `json:"domain,omitempty"`
	AllowDownload  bool     `json:"allow_download"`
}

type ShareResponse struct {
	ShareLink  string `json:"share_link"`
	Permission string `json:"permission"`
}
