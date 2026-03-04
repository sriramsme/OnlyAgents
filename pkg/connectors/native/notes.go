package native

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

type NotesConnector struct {
	store storage.NoteStore
	name  string
}

func NewNotesConnector(store storage.NoteStore) connectors.Connector {
	return &NotesConnector{
		store: store,
		name:  "native_notes",
	}
}

// ====================
// Connector Interface
// ====================

func (g *NotesConnector) Name() string { return g.name }
func (g *NotesConnector) Type() string { return "notes" }

func (g *NotesConnector) Connect() error {
	return nil
}

func (g *NotesConnector) Disconnect() error {
	return nil
}

func (g *NotesConnector) Start() error {
	return nil
}

func (g *NotesConnector) Stop() error {
	return nil
}

func (g *NotesConnector) HealthCheck() error {
	return nil
}

// createOne is internal — used by CreateNotes.
func (n *NotesConnector) createOne(ctx context.Context, note storage.Note) (*storage.Note, error) {
	if note.Title == "" {
		return nil, fmt.Errorf("notes: title is required")
	}

	now := storage.DBTime{Time: time.Now()}
	note.ID = uuid.NewString()
	note.CreatedAt = now
	note.UpdatedAt = now

	if err := n.store.CreateNote(ctx, &note); err != nil {
		return nil, err
	}

	return &note, nil
}

// CreateNotes is the public batch method.
// Returns all created notes and a slice of errors for failures.
func (n *NotesConnector) CreateNotes(ctx context.Context, notes []*storage.Note) ([]*storage.Note, []error) {
	results := make([]*storage.Note, 0, len(notes))
	var errs []error

	for _, note := range notes {
		created, err := n.createOne(ctx, *note)
		if err != nil {
			errs = append(errs, fmt.Errorf("note %q: %w", note.Title, err))
			continue
		}
		results = append(results, created)
	}

	return results, errs
}

func (n *NotesConnector) GetNote(ctx context.Context, id string) (*storage.Note, error) {
	return n.store.GetNote(ctx, id)
}

func (n *NotesConnector) UpdateNote(ctx context.Context, note *storage.Note) (*storage.Note, error) {
	if err := n.store.UpdateNote(ctx, note); err != nil {
		return nil, err
	}
	return n.store.GetNote(ctx, note.ID)
}

func (n *NotesConnector) DeleteNote(ctx context.Context, id string) error {
	return n.store.DeleteNote(ctx, id)
}

func (n *NotesConnector) ListNotes(ctx context.Context) ([]*storage.Note, error) {
	return n.store.ListNotes(ctx)
}

func (n *NotesConnector) SearchNotes(ctx context.Context, query string) ([]*storage.Note, error) {
	if query == "" {
		return n.store.ListNotes(ctx)
	}
	return n.store.SearchNotes(ctx, query)
}

func (n *NotesConnector) PinNote(ctx context.Context, id string, pinned bool) error {
	note, err := n.store.GetNote(ctx, id)
	if err != nil {
		return err
	}
	note.Pinned = pinned
	return n.store.UpdateNote(ctx, note)
}
