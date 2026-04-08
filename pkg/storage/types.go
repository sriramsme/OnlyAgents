package storage

// TopicEntry is a topic extracted from a day's conversation along with
// its relative share of the day's message volume and the user's sentiment.
type TopicEntry struct {
	Topic        string  `json:"topic"`
	MessageShare float64 `json:"message_share"` // 0.0 – 1.0
	Sentiment    string  `json:"sentiment"`     // "enthusiastic", "neutral", "frustrated", etc.
}

// DailySummary is an LLM-generated compression of all messages from one day.
type DailySummary struct {
	ID              string                `db:"id" json:"id"`
	Date            DBTime                `db:"date" json:"date"`
	Summary         string                `db:"summary" json:"summary,omitempty"`
	KeyEvents       JSONSlice[string]     `db:"key_events" json:"key_events,omitempty"`
	Topics          JSONSlice[TopicEntry] `db:"topics" json:"topics,omitempty"`
	ConversationIDs JSONSlice[string]     `db:"conversation_ids" json:"conversation_ids,omitempty"`
}

// WeeklySummary compresses daily summaries for one week.
type WeeklySummary struct {
	ID           string            `db:"id" json:"id"`
	WeekStart    DBTime            `db:"week_start" json:"week_start"`
	WeekEnd      DBTime            `db:"week_end" json:"week_end"`
	Summary      string            `db:"summary" json:"summary,omitempty"`
	Themes       JSONSlice[string] `db:"themes" json:"themes,omitempty"`
	Achievements JSONSlice[string] `db:"achievements" json:"achievements,omitempty"`
}

// MonthlySummary compresses weekly summaries for one month.
type MonthlySummary struct {
	ID         string            `db:"id" json:"id"`
	Year       int               `db:"year" json:"year"`
	Month      int               `db:"month" json:"month"`
	Summary    string            `db:"summary" json:"summary,omitempty"`
	Highlights JSONSlice[string] `db:"highlights" json:"highlights,omitempty"`
	Statistics JSONMap           `db:"statistics" json:"statistics,omitempty"`
}

// YearlyArchive is the final compression tier — kept forever.
type YearlyArchive struct {
	ID          string            `db:"id" json:"id"`
	Year        int               `db:"year" json:"year"`
	Summary     string            `db:"summary" json:"summary,omitempty"`
	MajorEvents JSONSlice[string] `db:"major_events" json:"major_events,omitempty"`
	Statistics  JSONMap           `db:"statistics" json:"statistics,omitempty"`
}

// Fact is a persistent piece of knowledge about an entity extracted during summarisation.
// SupersededBy points to a newer conflicting fact's ID, or "".
type Fact struct {
	ID                   string  `db:"id" json:"id"`
	Entity               string  `db:"entity" json:"entity"`
	EntityType           string  `db:"entity_type" json:"entity_type,omitempty"`
	Fact                 string  `db:"fact" json:"fact"`
	Confidence           float64 `db:"confidence" json:"confidence,omitempty"`
	TimesSeen            int     `db:"times_seen" json:"times_seen,omitempty"`
	SourceConversationID string  `db:"source_conversation_id" json:"source_conversation_id,omitempty"`
	SourceSummaryDate    string  `db:"source_summary_date" json:"source_summary_date,omitempty"`
	SupersededBy         string  `db:"superseded_by" json:"superseded_by,omitempty"`
	FirstSeen            DBTime  `db:"first_seen" json:"first_seen"`
	LastConfirmed        DBTime  `db:"last_confirmed" json:"last_confirmed"`
}
