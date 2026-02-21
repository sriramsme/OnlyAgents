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

// ====================
// Calendar Types
// ====================

type CalendarEvent struct {
	ID             string          `json:"id,omitempty"`
	CalendarID     string          `json:"calendar_id,omitempty"`
	Summary        string          `json:"summary"`
	Description    string          `json:"description,omitempty"`
	Location       string          `json:"location,omitempty"`
	Start          time.Time       `json:"start"`
	End            time.Time       `json:"end"`
	AllDay         bool            `json:"all_day"`
	Attendees      []EventAttendee `json:"attendees,omitempty"`
	Reminders      []EventReminder `json:"reminders,omitempty"`
	Recurring      bool            `json:"recurring"`
	RecurrenceRule string          `json:"recurrence_rule,omitempty"`
	Status         string          `json:"status,omitempty"`     // confirmed, tentative, cancelled
	Visibility     string          `json:"visibility,omitempty"` // public, private
	CreatedAt      time.Time       `json:"created_at,omitempty"`
	UpdatedAt      time.Time       `json:"updated_at,omitempty"`
	Raw            interface{}     `json:"raw,omitempty"`
}

type EventAttendee struct {
	Email          string `json:"email"`
	Name           string `json:"name,omitempty"`
	ResponseStatus string `json:"response_status,omitempty"` // accepted, declined, tentative, needsAction
	Optional       bool   `json:"optional"`
}

type EventReminder struct {
	Method  string `json:"method"` // email, popup, sms
	Minutes int    `json:"minutes"`
}

type ListEventsRequest struct {
	CalendarID string    `json:"calendar_id,omitempty"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	MaxResults int       `json:"max_results,omitempty"`
	Query      string    `json:"query,omitempty"`
}

type FindSlotsRequest struct {
	Duration     time.Duration `json:"duration"`
	Start        time.Time     `json:"start"`
	End          time.Time     `json:"end"`
	Attendees    []string      `json:"attendees,omitempty"`
	WorkingHours *WorkingHours `json:"working_hours,omitempty"`
}

type WorkingHours struct {
	StartHour int   `json:"start_hour"` // 0-23
	EndHour   int   `json:"end_hour"`   // 0-23
	Weekdays  []int `json:"weekdays"`   // 0=Sunday, 6=Saturday
}

type TimeSlot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
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
// Task Types
// ====================

type Task struct {
	ID          string      `json:"id,omitempty"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Status      string      `json:"status"`             // pending, in_progress, completed
	Priority    string      `json:"priority,omitempty"` // low, medium, high
	DueDate     *time.Time  `json:"due_date,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	ProjectID   string      `json:"project_id,omitempty"`
	ParentID    string      `json:"parent_id,omitempty"`
	Completed   bool        `json:"completed"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	CreatedAt   time.Time   `json:"created_at,omitempty"`
	UpdatedAt   time.Time   `json:"updated_at,omitempty"`
	Raw         interface{} `json:"raw,omitempty"`
}

type ListTasksRequest struct {
	ProjectID  string `json:"project_id,omitempty"`
	Status     string `json:"status,omitempty"`
	Completed  *bool  `json:"completed,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
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

// ====================
// Notes Types
// ====================

type Note struct {
	ID        string            `json:"id,omitempty"`
	Title     string            `json:"title"`
	Content   string            `json:"content"`
	Format    string            `json:"format"` // markdown, html, plain
	Tags      []string          `json:"tags,omitempty"`
	FolderID  string            `json:"folder_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at,omitempty"`
	UpdatedAt time.Time         `json:"updated_at,omitempty"`
	Raw       interface{}       `json:"raw,omitempty"`
}
