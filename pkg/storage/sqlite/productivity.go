package sqlite

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// ── CalendarStore ─────────────────────────────────────────────────────────────

func (d *DB) CreateEvent(ctx context.Context, event *storage.CalendarEvent) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO calendar_events
			(id, agent_id, title, description, start_time, end_time,
			 all_day, location, recurrence, tags, created_at, updated_at)
		VALUES
			(:id, :agent_id, :title, :description, :start_time, :end_time,
			 :all_day, :location, :recurrence, :tags, :created_at, :updated_at)
	`, event)
	return wrap(err, "create event")
}

func (d *DB) GetEvent(ctx context.Context, id string) (*storage.CalendarEvent, error) {
	var e storage.CalendarEvent
	err := d.db.GetContext(ctx, &e, `SELECT * FROM calendar_events WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get event")
	}
	return &e, nil
}

func (d *DB) UpdateEvent(ctx context.Context, event *storage.CalendarEvent) error {
	event.UpdatedAt = storage.DBTime{Time: time.Now()}
	_, err := d.db.NamedExecContext(ctx, `
		UPDATE calendar_events SET
			title       = :title,
			description = :description,
			start_time  = :start_time,
			end_time    = :end_time,
			all_day     = :all_day,
			location    = :location,
			recurrence  = :recurrence,
			tags        = :tags,
			updated_at  = :updated_at
		WHERE id = :id
	`, event)
	return wrap(err, "update event")
}

func (d *DB) DeleteEvent(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM calendar_events WHERE id = ?`, id)
	return wrap(err, "delete event")
}

func (d *DB) ListEvents(ctx context.Context, agentID string, from, to time.Time) ([]*storage.CalendarEvent, error) {
	var events []*storage.CalendarEvent
	err := d.db.SelectContext(ctx, &events, `
		SELECT * FROM calendar_events
		WHERE agent_id = ? AND start_time >= ? AND start_time <= ?
		ORDER BY start_time ASC
	`, agentID, storage.DBTime{Time: from}, storage.DBTime{Time: to})
	return events, wrap(err, "list events")
}

func (d *DB) GetUpcomingEvents(ctx context.Context, agentID string, limit int) ([]*storage.CalendarEvent, error) {
	var events []*storage.CalendarEvent
	err := d.db.SelectContext(ctx, &events, `
		SELECT * FROM calendar_events
		WHERE agent_id = ? AND start_time >= ?
		ORDER BY start_time ASC
		LIMIT ?
	`, agentID, storage.DBTime{Time: time.Now()}, limit)
	return events, wrap(err, "get upcoming events")
}

// ── NoteStore ─────────────────────────────────────────────────────────────────

func (d *DB) CreateNote(ctx context.Context, note *storage.Note) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO notes (id, agent_id, title, content, tags, pinned, created_at, updated_at)
		VALUES (:id, :agent_id, :title, :content, :tags, :pinned, :created_at, :updated_at)
	`, note)
	return wrap(err, "create note")
}

func (d *DB) GetNote(ctx context.Context, id string) (*storage.Note, error) {
	var n storage.Note
	err := d.db.GetContext(ctx, &n, `SELECT * FROM notes WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get note")
	}
	return &n, nil
}

func (d *DB) UpdateNote(ctx context.Context, note *storage.Note) error {
	note.UpdatedAt = storage.DBTime{Time: time.Now()}
	_, err := d.db.NamedExecContext(ctx, `
		UPDATE notes
		SET title = :title, content = :content, tags = :tags,
		    pinned = :pinned, updated_at = :updated_at
		WHERE id = :id
	`, note)
	return wrap(err, "update note")
}

func (d *DB) DeleteNote(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	return wrap(err, "delete note")
}

func (d *DB) ListNotes(ctx context.Context, agentID string) ([]*storage.Note, error) {
	var notes []*storage.Note
	err := d.db.SelectContext(ctx, &notes, `
		SELECT * FROM notes WHERE agent_id = ? ORDER BY pinned DESC, updated_at DESC
	`, agentID)
	return notes, wrap(err, "list notes")
}

// SearchNotes uses FTS5 to search across note title and content.
func (d *DB) SearchNotes(ctx context.Context, agentID string, query string) ([]*storage.Note, error) {
	var notes []*storage.Note
	err := d.db.SelectContext(ctx, &notes, `
		SELECT n.* FROM notes n
		INNER JOIN notes_fts ON n.rowid = notes_fts.rowid
		WHERE notes_fts MATCH ? AND n.agent_id = ?
		ORDER BY rank
	`, query, agentID)
	return notes, wrap(err, "search notes")
}

// ── ReminderStore ─────────────────────────────────────────────────────────────

func (d *DB) CreateReminder(ctx context.Context, r *storage.Reminder) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO reminders (id, agent_id, title, body, due_at, sent_at, recurring, created_at)
		VALUES (:id, :agent_id, :title, :body, :due_at, :sent_at, :recurring, :created_at)
	`, r)
	return wrap(err, "create reminder")
}

func (d *DB) GetReminder(ctx context.Context, id string) (*storage.Reminder, error) {
	var r storage.Reminder
	err := d.db.GetContext(ctx, &r, `SELECT * FROM reminders WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get reminder")
	}
	return &r, nil
}

func (d *DB) UpdateReminder(ctx context.Context, r *storage.Reminder) error {
	_, err := d.db.NamedExecContext(ctx, `
		UPDATE reminders
		SET title = :title, body = :body, due_at = :due_at, recurring = :recurring
		WHERE id = :id
	`, r)
	return wrap(err, "update reminder")
}

func (d *DB) DeleteReminder(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM reminders WHERE id = ?`, id)
	return wrap(err, "delete reminder")
}

func (d *DB) ListReminders(ctx context.Context, agentID string) ([]*storage.Reminder, error) {
	var reminders []*storage.Reminder
	err := d.db.SelectContext(ctx, &reminders, `
		SELECT * FROM reminders WHERE agent_id = ? AND sent_at IS NULL ORDER BY due_at ASC
	`, agentID)
	return reminders, wrap(err, "list reminders")
}

func (d *DB) GetDueReminders(ctx context.Context, before time.Time) ([]*storage.Reminder, error) {
	var reminders []*storage.Reminder
	err := d.db.SelectContext(ctx, &reminders, `
		SELECT * FROM reminders WHERE due_at <= ? AND sent_at IS NULL
	`, storage.DBTime{Time: before})
	return reminders, wrap(err, "get due reminders")
}

func (d *DB) MarkReminderSent(ctx context.Context, id string) error {
	now := storage.DBTime{Time: time.Now()}
	_, err := d.db.ExecContext(ctx, `UPDATE reminders SET sent_at = ? WHERE id = ?`, now, id)
	return wrap(err, "mark reminder sent")
}
