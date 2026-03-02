-- Hierarchical memory compression tables.

CREATE TABLE IF NOT EXISTS daily_summaries (
    id               TEXT PRIMARY KEY,
    agent_id         TEXT NOT NULL,
    date             TEXT NOT NULL,          -- stored as YYYY-MM-DD
    summary          TEXT NOT NULL DEFAULT '',
    key_events       TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    topics           TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    conversation_ids TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    UNIQUE (agent_id, date)
);

CREATE TABLE IF NOT EXISTS weekly_summaries (
    id           TEXT PRIMARY KEY,
    agent_id     TEXT NOT NULL,
    week_start   TEXT NOT NULL,
    week_end     TEXT NOT NULL,
    summary      TEXT NOT NULL DEFAULT '',
    themes       TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    achievements TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    UNIQUE (agent_id, week_start)
);

CREATE TABLE IF NOT EXISTS monthly_summaries (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL,
    year       INTEGER NOT NULL,
    month      INTEGER NOT NULL,
    summary    TEXT NOT NULL DEFAULT '',
    highlights TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    statistics TEXT NOT NULL DEFAULT '{}', -- JSONMap
    UNIQUE (agent_id, year, month)
);

CREATE TABLE IF NOT EXISTS yearly_archives (
    id           TEXT PRIMARY KEY,
    agent_id     TEXT NOT NULL,
    year         INTEGER NOT NULL,
    summary      TEXT NOT NULL DEFAULT '',
    major_events TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    statistics   TEXT NOT NULL DEFAULT '{}', -- JSONMap
    UNIQUE (agent_id, year)
);

-- Facts: persistent entity knowledge extracted during summarisation.

CREATE TABLE IF NOT EXISTS facts (
    id                     TEXT PRIMARY KEY,
    agent_id               TEXT NOT NULL,
    entity                 TEXT NOT NULL,
    entity_type            TEXT NOT NULL DEFAULT '',
    fact                   TEXT NOT NULL,
    confidence             REAL NOT NULL DEFAULT 1.0,
    source_conversation_id TEXT NOT NULL DEFAULT '',
    superseded_by          TEXT NOT NULL DEFAULT '', -- ID of newer fact, or ''
    first_seen             TEXT NOT NULL,
    last_confirmed         TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_facts_agent_entity ON facts (agent_id, entity);

-- FTS5 index for SearchFacts. Triggers keep it in sync automatically.
CREATE VIRTUAL TABLE IF NOT EXISTS facts_fts USING fts5 (
    fact, entity,
    content=facts, content_rowid=rowid
);

CREATE TRIGGER IF NOT EXISTS facts_ai AFTER INSERT ON facts BEGIN
    INSERT INTO facts_fts (rowid, fact, entity)
    VALUES (new.rowid, new.fact, new.entity);
END;

CREATE TRIGGER IF NOT EXISTS facts_ad AFTER DELETE ON facts BEGIN
    INSERT INTO facts_fts (facts_fts, rowid, fact, entity)
    VALUES ('delete', old.rowid, old.fact, old.entity);
END;

CREATE TRIGGER IF NOT EXISTS facts_au AFTER UPDATE ON facts BEGIN
    INSERT INTO facts_fts (facts_fts, rowid, fact, entity)
    VALUES ('delete', old.rowid, old.fact, old.entity);
    INSERT INTO facts_fts (rowid, fact, entity)
    VALUES (new.rowid, new.fact, new.entity);
END;
