-- Native productivity tables: calendar, notes, reminders.

CREATE TABLE IF NOT EXISTS calendar_events (
    id          TEXT PRIMARY KEY,
    agent_id    TEXT NOT NULL,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    start_time  TEXT NOT NULL,
    end_time    TEXT NOT NULL,
    all_day     INTEGER NOT NULL DEFAULT 0,  -- 0 = false, 1 = true
    location    TEXT NOT NULL DEFAULT '',
    recurrence  TEXT NOT NULL DEFAULT '',    -- RRULE string or ''
    tags        TEXT NOT NULL DEFAULT '[]',  -- JSONSlice[string]
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_calendar_agent_time
    ON calendar_events (agent_id, start_time ASC);

-- Notes (Markdown content, FTS-indexed).

CREATE TABLE IF NOT EXISTS notes (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL,
    title      TEXT NOT NULL,
    content    TEXT NOT NULL DEFAULT '', -- Markdown
    tags       TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    pinned     INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notes_agent
    ON notes (agent_id, updated_at DESC);

CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5 (
    title, content,
    content=notes, content_rowid=rowid
);

CREATE TRIGGER IF NOT EXISTS notes_ai AFTER INSERT ON notes BEGIN
    INSERT INTO notes_fts (rowid, title, content)
    VALUES (new.rowid, new.title, new.content);
END;

CREATE TRIGGER IF NOT EXISTS notes_ad AFTER DELETE ON notes BEGIN
    INSERT INTO notes_fts (notes_fts, rowid, title, content)
    VALUES ('delete', old.rowid, old.title, old.content);
END;

CREATE TRIGGER IF NOT EXISTS notes_au AFTER UPDATE ON notes BEGIN
    INSERT INTO notes_fts (notes_fts, rowid, title, content)
    VALUES ('delete', old.rowid, old.title, old.content);
    INSERT INTO notes_fts (rowid, title, content)
    VALUES (new.rowid, new.title, new.content);
END;

-- Reminders.

CREATE TABLE IF NOT EXISTS reminders (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    due_at     TEXT NOT NULL,
    sent_at    TEXT,                         -- NULL until delivered
    recurring  TEXT NOT NULL DEFAULT '',     -- RRULE or ''
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_reminders_due
    ON reminders (due_at ASC, sent_at);
