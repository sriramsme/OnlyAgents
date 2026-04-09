package memory

import (
	"context"
	"time"
)

type EpisodeStore interface {
	SaveEpisode(ctx context.Context, e *Episode) error
	GetEpisode(ctx context.Context, id string) (*Episode, error)
	SearchEpisodes(ctx context.Context, q EpisodeQuery) ([]*Episode, error)
	GetEpisodesByScope(ctx context.Context, scope EpisodeScope, from, to time.Time) ([]*Episode, error)
	PruneEpisodes(ctx context.Context, before time.Time, maxImportance float32) (int, error)
}

type NexusStore interface {
	UpsertEntity(ctx context.Context, e *Entity) (*Entity, error)
	FindSimilarEntities(ctx context.Context, name string) ([]*Entity, error) // FTS candidate lookup
	SaveRelation(ctx context.Context, r *Relation) error
	InvalidateRelation(ctx context.Context, id string, endedAt time.Time) error
	QueryEntity(ctx context.Context, entityID string, asOf *time.Time) ([]*Relation, error)
	Timeline(ctx context.Context, entityID string) ([]*Relation, error)
	LinkEpisodeEntities(ctx context.Context, episodeID string, entityIDs []string) error
	AddAlias(ctx context.Context, entityID, alias, sourceEpisodeID string) error
}

type PraxisStore interface {
	SavePattern(ctx context.Context, p *Pattern) error
	UpdatePattern(ctx context.Context, id string, confidence float32, lastSeen time.Time) error
	SearchPatterns(ctx context.Context, embedding []float32, limit int) ([]*Pattern, error)
	DecayStalePatterns(ctx context.Context, staleBefore time.Time, decayFactor float32) error
	GetAllPatterns(ctx context.Context) ([]*Pattern, error)
}

// EpisodeScope defines the temporal granularity of an episode.
type EpisodeScope string

const (
	ScopeSession EpisodeScope = "session"
	ScopeDaily   EpisodeScope = "daily"
	ScopeWeekly  EpisodeScope = "weekly"
	ScopeMonthly EpisodeScope = "monthly"
	ScopeYearly  EpisodeScope = "yearly"
)

type Episode struct {
	ID         string
	Scope      EpisodeScope
	Summary    string
	Embedding  []float32
	Importance float32
	StartedAt  time.Time
	EndedAt    time.Time
	CreatedAt  time.Time
}

type EpisodeQuery struct {
	Scope     *EpisodeScope
	From      *time.Time
	To        *time.Time
	Embedding []float32 // if set, do vector/FTS search
	Limit     int
}

type EntityType string

const (
	EntityPerson     EntityType = "person"
	EntityProject    EntityType = "project"
	EntityTool       EntityType = "tool"
	EntityConcept    EntityType = "concept"
	EntityDecision   EntityType = "decision"
	EntityPreference EntityType = "preference"
)

type Entity struct {
	ID            string
	CanonicalName string
	Type          EntityType
	CreatedAt     time.Time
}

type Relation struct {
	ID              string
	SubjectID       string
	Predicate       string
	ObjectID        *string // nil if object is a literal
	ObjectLiteral   *string // nil if object is an entity
	Confidence      float32
	ValidFrom       time.Time
	ValidUntil      *time.Time // nil = currently true
	SourceEpisodeID *string
	CreatedAt       time.Time
}

type Pattern struct {
	ID               string
	Description      string
	Embedding        []float32
	Confidence       float32
	ObservationCount int
	FirstObservedAt  time.Time
	LastObservedAt   time.Time
	CreatedAt        time.Time
}
