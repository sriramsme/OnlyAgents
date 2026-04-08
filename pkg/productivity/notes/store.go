package notes

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
)

type Store interface {
	CreateNote(ctx context.Context, note *Note) error
	GetNote(ctx context.Context, id string) (*Note, error)
	UpdateNote(ctx context.Context, note *Note) error
	DeleteNote(ctx context.Context, id string) error
	ListNotes(ctx context.Context) ([]*Note, error)
	SearchNotes(ctx context.Context, query string) ([]*Note, error)
}

// Note is a Markdown note.
type Note struct {
	ID        string                    `db:"id" json:"id"`
	Title     string                    `db:"title" json:"title"`
	Content   string                    `db:"content" json:"content,omitempty"`
	Tags      dbtypes.JSONSlice[string] `db:"tags" json:"tags,omitempty"`
	Pinned    bool                      `db:"pinned" json:"pinned,omitempty"`
	CreatedAt dbtypes.DBTime            `db:"created_at" json:"created_at"`
	UpdatedAt dbtypes.DBTime            `db:"updated_at" json:"updated_at"`
}
