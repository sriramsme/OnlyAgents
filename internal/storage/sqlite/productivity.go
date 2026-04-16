package sqlite

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/calendar"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/notes"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/reminder"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/task"
)

// ── CalendarStore ─────────────────────────────────────────────────────────────

func (d *DB) CreateEvent(ctx context.Context, event *calendar.CalendarEvent) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO calendar_events
			(id, title, description, start_time, end_time,
			 all_day, location, recurrence, tags, created_at, updated_at)
		VALUES
			(:id, :title, :description, :start_time, :end_time,
			 :all_day, :location, :recurrence, :tags, :created_at, :updated_at)
	`, event)
	return wrap(err, "create event")
}

func (d *DB) GetEvent(ctx context.Context, id string) (*calendar.CalendarEvent, error) {
	var e calendar.CalendarEvent
	err := d.db.GetContext(ctx, &e, `SELECT * FROM calendar_events WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get event")
	}
	return &e, nil
}

func (d *DB) UpdateEvent(ctx context.Context, event *calendar.CalendarEvent) error {
	event.UpdatedAt = dbtypes.DBTime{Time: time.Now()}
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

func (d *DB) ListEvents(ctx context.Context, from, to time.Time) ([]*calendar.CalendarEvent, error) {
	var events []*calendar.CalendarEvent
	err := d.db.SelectContext(ctx, &events, `
		SELECT * FROM calendar_events
		WHERE start_time >= ? AND start_time <= ?
		ORDER BY start_time ASC
	`, dbtypes.DBTime{Time: from}, dbtypes.DBTime{Time: to})
	return events, wrap(err, "list events")
}

func (d *DB) GetUpcomingEvents(ctx context.Context, limit int) ([]*calendar.CalendarEvent, error) {
	var events []*calendar.CalendarEvent
	err := d.db.SelectContext(ctx, &events, `
		SELECT * FROM calendar_events
		WHERE start_time >= ?
		ORDER BY start_time ASC
		LIMIT ?
	`, dbtypes.DBTime{Time: time.Now()}, limit)
	return events, wrap(err, "get upcoming events")
}

func (d *DB) SearchEvents(ctx context.Context, query string, limit int) ([]*calendar.CalendarEvent, error) {
	const q = `
        SELECT * FROM calendar_events
        WHERE title       LIKE '%' || ? || '%'
           OR description LIKE '%' || ? || '%'
           OR location    LIKE '%' || ? || '%'
        ORDER BY ABS(strftime('%s', start_time) - strftime('%s', 'now')) ASC
        LIMIT ?`

	type row struct {
		calendar.CalendarEvent
	}
	var rows []row
	if err := d.db.SelectContext(ctx, &rows, q, query, query, query, limit); err != nil {
		return nil, wrap(err, "search events")
	}
	events := make([]*calendar.CalendarEvent, len(rows))
	for i := range rows {
		e := rows[i].CalendarEvent
		events[i] = &e
	}
	return events, nil
}

// ── NoteStore ─────────────────────────────────────────────────────────────────

func (d *DB) CreateNote(ctx context.Context, note *notes.Note) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO notes (id, title, content, tags, pinned, created_at, updated_at)
		VALUES (:id, :title, :content, :tags, :pinned, :created_at, :updated_at)
	`, note)
	return wrap(err, "create note")
}

func (d *DB) GetNote(ctx context.Context, id string) (*notes.Note, error) {
	var n notes.Note
	err := d.db.GetContext(ctx, &n, `SELECT * FROM notes WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get note")
	}
	return &n, nil
}

func (d *DB) UpdateNote(ctx context.Context, note *notes.Note) error {
	note.UpdatedAt = dbtypes.DBTime{Time: time.Now()}
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

func (d *DB) ListNotes(ctx context.Context) ([]*notes.Note, error) {
	var notes []*notes.Note
	err := d.db.SelectContext(ctx, &notes, `
		SELECT * FROM notes ORDER BY pinned DESC, updated_at DESC
	`)
	return notes, wrap(err, "list notes")
}

func (d *DB) SearchNotes(ctx context.Context, query string) ([]*notes.Note, error) {
	const q = `
        SELECT n.*
        FROM notes n
        JOIN notes_fts fts ON n.rowid = fts.rowid
        WHERE notes_fts MATCH ?
        ORDER BY n.pinned DESC, rank, n.updated_at DESC`

	type row struct{ notes.Note }
	var rows []row
	if err := d.db.SelectContext(ctx, &rows, q, query); err != nil {
		return nil, wrap(err, "search notes")
	}
	result := make([]*notes.Note, len(rows))
	for i := range rows {
		n := rows[i].Note
		result[i] = &n
	}
	return result, nil
}

// ── ReminderStore ─────────────────────────────────────────────────────────────

func (d *DB) CreateReminder(ctx context.Context, r *reminder.Reminder) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO reminders (id, title, body, due_at, sent_at, recurring, created_at)
		VALUES (:id, :title, :body, :due_at, :sent_at, :recurring, :created_at)
	`, r)
	return wrap(err, "create reminder")
}

func (d *DB) GetReminder(ctx context.Context, id string) (*reminder.Reminder, error) {
	var r reminder.Reminder
	err := d.db.GetContext(ctx, &r, `SELECT * FROM reminders WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get reminder")
	}
	return &r, nil
}

func (d *DB) UpdateReminder(ctx context.Context, r *reminder.Reminder) error {
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

func (d *DB) ListReminders(ctx context.Context) ([]*reminder.Reminder, error) {
	var reminders []*reminder.Reminder
	err := d.db.SelectContext(ctx, &reminders, `
		SELECT * FROM reminders ORDER BY due_at ASC
	`)
	return reminders, wrap(err, "list reminders")
}

func (d *DB) GetDueReminders(ctx context.Context, before time.Time) ([]*reminder.Reminder, error) {
	var reminders []*reminder.Reminder
	err := d.db.SelectContext(ctx, &reminders, `
		SELECT * FROM reminders WHERE due_at <= ? AND sent_at IS NULL
	`, dbtypes.DBTime{Time: before})
	return reminders, wrap(err, "get due reminders")
}

func (d *DB) MarkReminderSent(ctx context.Context, id string, sentAt time.Time) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE reminders SET sent_at = ? WHERE id = ?`,
		dbtypes.DBTime{Time: sentAt}, id)
	return wrap(err, "mark reminder sent")
}

// ── ProjectStore ──────────────────────────────────────────────────────────────

func (d *DB) CreateProject(ctx context.Context, project *task.Project) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO projects (id, name, description, color, created_at, updated_at)
		VALUES (:id, :name, :description, :color, :created_at, :updated_at)
	`, project)
	return wrap(err, "create project")
}

func (d *DB) GetProject(ctx context.Context, id string) (*task.Project, error) {
	var p task.Project
	err := d.db.GetContext(ctx, &p, `SELECT * FROM projects WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get project")
	}
	return &p, nil
}

func (d *DB) UpdateProject(ctx context.Context, project *task.Project) error {
	project.UpdatedAt = dbtypes.DBTime{Time: time.Now()}
	_, err := d.db.NamedExecContext(ctx, `
		UPDATE projects
		SET name = :name, description = :description, color = :color, updated_at = :updated_at
		WHERE id = :id
	`, project)
	return wrap(err, "update project")
}

func (d *DB) DeleteProject(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	return wrap(err, "delete project")
}

func (d *DB) ListProjects(ctx context.Context) ([]*task.Project, error) {
	var projects []*task.Project
	err := d.db.SelectContext(ctx, &projects,
		`SELECT * FROM projects ORDER BY name ASC`)
	return projects, wrap(err, "list projects")
}

// ── TaskStore ─────────────────────────────────────────────────────────────────

func (d *DB) CreateTask(ctx context.Context, task *task.Task) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO tasks
			(id, project_id, title, body, status, priority,
			 due_at, completed_at, tags, created_at, updated_at)
		VALUES
			(:id, :project_id, :title, :body, :status, :priority,
			 :due_at, :completed_at, :tags, :created_at, :updated_at)
	`, task)
	return wrap(err, "create task")
}

func (d *DB) GetTask(ctx context.Context, id string) (*task.Task, error) {
	var t task.Task
	err := d.db.GetContext(ctx, &t, `SELECT * FROM tasks WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get task")
	}
	return &t, nil
}

func (d *DB) UpdateTask(ctx context.Context, task *task.Task) error {
	task.UpdatedAt = dbtypes.DBTime{Time: time.Now()}
	_, err := d.db.NamedExecContext(ctx, `
		UPDATE tasks SET
			project_id   = :project_id,
			title        = :title,
			body         = :body,
			status       = :status,
			priority     = :priority,
			due_at       = :due_at,
			completed_at = :completed_at,
			tags         = :tags,
			updated_at   = :updated_at
		WHERE id = :id
	`, task)
	return wrap(err, "update task")
}

func (d *DB) DeleteTask(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	return wrap(err, "delete task")
}

func (d *DB) CompleteTask(ctx context.Context, id string) error {
	now := dbtypes.DBTime{Time: time.Now()}
	_, err := d.db.ExecContext(ctx, `
		UPDATE tasks SET status = 'done', completed_at = ?, updated_at = ? WHERE id = ?
	`, now, now, id)
	return wrap(err, "complete task")
}

func (d *DB) ListTasks(ctx context.Context, filter task.TaskFilter) ([]*task.Task, error) {
	query := `SELECT * FROM tasks WHERE 1=1`
	args := []any{}

	if filter.ProjectID != nil {
		query += ` AND project_id = ?`
		args = append(args, *filter.ProjectID)
	}
	if filter.Status != nil {
		query += ` AND status = ?`
		args = append(args, *filter.Status)
	}
	if filter.Priority != nil {
		query += ` AND priority = ?`
		args = append(args, *filter.Priority)
	}
	if filter.DueFrom != nil {
		query += ` AND due_at >= ?`
		args = append(args, dbtypes.DBTime{Time: *filter.DueFrom})
	}
	if filter.DueTo != nil {
		query += ` AND due_at <= ?`
		args = append(args, dbtypes.DBTime{Time: *filter.DueTo})
	}

	query += ` ORDER BY
		CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END ASC,
		due_at ASC NULLS LAST`

	var tasks []*task.Task
	err := d.db.SelectContext(ctx, &tasks, query, args...)
	return tasks, wrap(err, "list tasks")
}

func (d *DB) SearchTasks(ctx context.Context, query string) ([]*task.Task, error) {
	const q = `
        SELECT t.*
        FROM tasks t
        JOIN tasks_fts fts ON t.rowid = fts.rowid
        WHERE tasks_fts MATCH ?
          AND t.status NOT IN ('done', 'cancelled')
        ORDER BY
            CASE t.priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END,
            t.due_at ASC NULLS LAST,
            rank`

	type row struct{ task.Task }
	var rows []row
	if err := d.db.SelectContext(ctx, &rows, q, query); err != nil {
		return nil, wrap(err, "search tasks")
	}
	result := make([]*task.Task, len(rows))
	for i := range rows {
		t := rows[i].Task
		result[i] = &t
	}
	return result, nil
}

func (d *DB) GetTasksDueOn(ctx context.Context, date time.Time) ([]*task.Task, error) {
	dayStart := truncateToDay(date)
	dayEnd := dayStart.Add(24*time.Hour - time.Second)
	var tasks []*task.Task
	err := d.db.SelectContext(ctx, &tasks, `
		SELECT * FROM tasks
		WHERE due_at >= ?
		  AND due_at <= ?
		  AND status NOT IN ('done', 'cancelled')
		ORDER BY
			CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END ASC
	`, dbtypes.DBTime{Time: dayStart}, dbtypes.DBTime{Time: dayEnd})
	return tasks, wrap(err, "get tasks due on")
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
