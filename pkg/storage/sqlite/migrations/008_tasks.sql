-- Projects/Tasks (To-do lists)
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    color       TEXT NOT NULL DEFAULT '#6366f1',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL DEFAULT '' REFERENCES projects(id) ON DELETE SET DEFAULT,
    title        TEXT NOT NULL,
    body         TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'todo',
    priority     TEXT NOT NULL DEFAULT 'medium',
    due_at       TEXT,
    completed_at TEXT,
    tags         TEXT NOT NULL DEFAULT '[]',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_status   ON tasks (status);
CREATE INDEX IF NOT EXISTS idx_tasks_project  ON tasks (project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_due      ON tasks (due_at);

CREATE VIRTUAL TABLE IF NOT EXISTS tasks_fts USING fts5 (
    title, body,
    content=tasks, content_rowid=rowid
);

CREATE TRIGGER IF NOT EXISTS tasks_ai AFTER INSERT ON tasks BEGIN
    INSERT INTO tasks_fts (rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;

CREATE TRIGGER IF NOT EXISTS tasks_ad AFTER DELETE ON tasks BEGIN
    INSERT INTO tasks_fts (tasks_fts, rowid, title, body)
    VALUES ('delete', old.rowid, old.title, old.body);
END;

CREATE TRIGGER IF NOT EXISTS tasks_au AFTER UPDATE ON tasks BEGIN
    INSERT INTO tasks_fts (tasks_fts, rowid, title, body)
    VALUES ('delete', old.rowid, old.title, old.body);
    INSERT INTO tasks_fts (rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;
