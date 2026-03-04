package notes

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

const version = "1.0.0"

func init() {
	skills.Register("notes", NewNotesSkill)
}

type NotesSkill struct {
	ctx    context.Context
	cancel context.CancelFunc
	*skills.BaseSkill
	conn connectors.NotesConnector
}

func NewNotesSkill(ctx context.Context, eventBus chan<- core.Event) (skills.Skill, error) {
	base := skills.NewBaseSkill(
		tools.SkillNotes,
		"Create, update, search, and manage Markdown notes",
		version,
		skills.SkillTypeNative,
	)
	skillCtx, cancel := context.WithCancel(ctx)
	return &NotesSkill{
		BaseSkill: base,
		ctx:       skillCtx,
		cancel:    cancel,
	}, nil
}

func (s *NotesSkill) Initialize(deps skills.SkillDeps) error {
	s.SetOutbox(deps.Outbox)
	for _, c := range deps.Connectors {
		if nc, ok := c.(connectors.NotesConnector); ok {
			s.conn = nc
			return nil
		}
	}
	logger.Log.Error("notes skill: no NotesConnector found in deps")
	return nil
}

func (s *NotesSkill) Shutdown() error {
	s.cancel()
	return nil
}

func (s *NotesSkill) RequiredCapabilities() []core.Capability {
	return []core.Capability{core.CapabilityNotes}
}

func (s *NotesSkill) Tools() []tools.ToolDef {
	return tools.GetNotesTools()
}

func (s *NotesSkill) Execute(ctx context.Context, toolName string, args []byte) (any, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("notes skill: connector not initialized")
	}
	switch toolName {
	case "notes_create":
		return s.createNotes(ctx, args)
	case "notes_update":
		return s.updateNote(ctx, args)
	case "notes_get":
		return s.getNote(ctx, args)
	case "notes_delete":
		return s.deleteNote(ctx, args)
	case "notes_list":
		return s.conn.ListNotes(ctx)
	case "notes_search":
		return s.searchNotes(ctx, args)
	case "notes_pin":
		return s.pinNote(ctx, args)
	default:
		return nil, fmt.Errorf("notes skill: unknown tool %q", toolName)
	}
}

func (s *NotesSkill) createNotes(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.NotesCreateInput](args)
	if err != nil {
		return nil, err
	}

	if len(input.Notes) == 0 {
		return nil, fmt.Errorf("notes: at least one note is required")
	}

	notes := make([]*storage.Note, 0, len(input.Notes))

	for _, item := range input.Notes {
		note := &storage.Note{
			Title:   item.Title,
			Content: item.Content,
			Tags:    item.Tags,
			Pinned:  item.Pinned,
		}
		notes = append(notes, note)
	}

	created, errs := s.conn.CreateNotes(ctx, notes)

	response := map[string]any{
		"created": created,
		"count":   len(created),
	}

	if len(errs) > 0 {
		errMsgs := make([]string, len(errs))
		for i, e := range errs {
			errMsgs[i] = e.Error()
		}
		response["errors"] = errMsgs
		response["failed_count"] = len(errs)
	}

	return response, nil
}

func (s *NotesSkill) updateNote(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.NotesUpdateInput](args)
	if err != nil {
		return nil, err
	}
	note, err := s.conn.GetNote(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("notes: %q not found: %w", input.ID, err)
	}
	if input.Title != "" {
		note.Title = input.Title
	}
	if input.Content != "" {
		note.Content = input.Content
	}
	if input.Tags != nil {
		note.Tags = input.Tags
	}
	return s.conn.UpdateNote(ctx, note)
}

func (s *NotesSkill) getNote(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.NotesGetInput](args)
	if err != nil {
		return nil, err
	}
	return s.conn.GetNote(ctx, input.ID)
}

func (s *NotesSkill) deleteNote(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.NotesDeleteInput](args)
	if err != nil {
		return nil, err
	}
	if err := s.conn.DeleteNote(ctx, input.ID); err != nil {
		return nil, err
	}
	return map[string]any{"status": "deleted", "id": input.ID}, nil
}

func (s *NotesSkill) searchNotes(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.NotesSearchInput](args)
	if err != nil {
		return nil, err
	}
	return s.conn.SearchNotes(ctx, input.Query)
}

func (s *NotesSkill) pinNote(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.NotesPinInput](args)
	if err != nil {
		return nil, err
	}
	if err := s.conn.PinNote(ctx, input.ID, input.Pinned); err != nil {
		return nil, err
	}
	return map[string]any{"status": "ok", "id": input.ID, "pinned": input.Pinned}, nil
}
