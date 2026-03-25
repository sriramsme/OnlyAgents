-- Hierarchical memory compression tables.
-- system-wide context, not scoped to individual agents.

CREATE TABLE IF NOT EXISTS daily_summaries (
    id               TEXT PRIMARY KEY,
    date             TEXT NOT NULL,              -- UTC start-of-local-day, full ISO timestamp via DBTime
    summary          TEXT NOT NULL DEFAULT '',
    key_events       TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    topics           TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    conversation_ids TEXT NOT NULL DEFAULT '[]', -- JSONSlice[string]
    UNIQUE (date)
);

CREATE TABLE IF NOT EXISTS weekly_summaries (
    id           TEXT PRIMARY KEY,
    week_start   TEXT NOT NULL,                  -- UTC, full ISO timestamp via DBTime
    week_end     TEXT NOT NULL,                  -- UTC, full ISO timestamp via DBTime
    summary      TEXT NOT NULL DEFAULT '',
    themes       TEXT NOT NULL DEFAULT '[]',     -- JSONSlice[string]
    achievements TEXT NOT NULL DEFAULT '[]',     -- JSONSlice[string]
    UNIQUE (week_start)
);

CREATE TABLE IF NOT EXISTS monthly_summaries (
    id         TEXT PRIMARY KEY,
    year       INTEGER NOT NULL,
    month      INTEGER NOT NULL,
    summary    TEXT NOT NULL DEFAULT '',
    highlights TEXT NOT NULL DEFAULT '[]',       -- JSONSlice[string]
    statistics TEXT NOT NULL DEFAULT '{}',       -- JSONMap
    UNIQUE (year, month)
);

CREATE TABLE IF NOT EXISTS yearly_archives (
    id           TEXT PRIMARY KEY,
    year         INTEGER NOT NULL,
    summary      TEXT NOT NULL DEFAULT '',
    major_events TEXT NOT NULL DEFAULT '[]',     -- JSONSlice[string]
    statistics   TEXT NOT NULL DEFAULT '{}',     -- JSONMap
    UNIQUE (year)
);

-- Facts: persistent entity knowledge extracted during summarisation.
-- superseded_by: ID of the newer fact that replaced this one, or '' if still active.
-- source_conversation_id: conversation from which this fact was extracted, or ''.
CREATE TABLE IF NOT EXISTS facts (
    id                   TEXT    PRIMARY KEY,
    entity               TEXT    NOT NULL,
    entity_type          TEXT    NOT NULL DEFAULT 'other',
    fact                 TEXT    NOT NULL,
    confidence           REAL    NOT NULL DEFAULT 1.0,
    times_seen           INTEGER NOT NULL DEFAULT 1,      -- incremented on each re-confirmation
    source_conversation_id TEXT  NOT NULL DEFAULT '',
    source_summary_date  TEXT    NOT NULL DEFAULT '',     -- YYYY-MM-DD
    superseded_by        TEXT    NOT NULL DEFAULT '',
    first_seen           TEXT    NOT NULL,
    last_confirmed       TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_facts_entity            ON facts (entity);
CREATE INDEX IF NOT EXISTS idx_facts_entity_type       ON facts (entity_type);
CREATE INDEX IF NOT EXISTS idx_facts_last_confirmed    ON facts (last_confirmed);

CREATE TRIGGER IF NOT EXISTS facts_ai AFTER INSERT ON facts BEGIN
    INSERT INTO facts_fts (rowid, fact, entity)
    VALUES (new.rowid, new.fact, new.entity);
END;

-- FTS5 index for SearchFacts over active facts only.
-- Triggers keep it in sync automatically.
CREATE VIRTUAL TABLE IF NOT EXISTS facts_fts USING fts5 (
    fact,
    entity,
    content=facts,
    content_rowid=rowid
);

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

CREATE INDEX IF NOT EXISTS idx_daily_date          ON daily_summaries (date);
CREATE INDEX IF NOT EXISTS idx_weekly_start        ON weekly_summaries (week_start);
CREATE INDEX IF NOT EXISTS idx_monthly_year_month  ON monthly_summaries (year, month);
CREATE INDEX IF NOT EXISTS idx_yearly_year         ON yearly_archives (year);
