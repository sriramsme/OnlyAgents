CREATE TABLE episodes (
    id           TEXT PRIMARY KEY,
    scope        TEXT NOT NULL CHECK (scope IN ('session','daily','weekly','monthly','yearly')),
    summary      TEXT NOT NULL,
    embedding    BLOB,                  -- float32 vector, NULL if no embedder configured
    importance   REAL NOT NULL DEFAULT 0.5,
    started_at   DATETIME NOT NULL,
    ended_at     DATETIME NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- FTS over summaries for fallback search when no embedder
CREATE VIRTUAL TABLE episodes_fts USING fts5(
    summary,
    content=episodes,
    content_rowid=rowid
);

CREATE INDEX idx_episodes_scope_time ON episodes(scope, started_at, ended_at);
CREATE INDEX idx_episodes_importance ON episodes(importance);


-- NEXUS

CREATE TABLE entities (
    id             TEXT PRIMARY KEY,
    canonical_name TEXT NOT NULL,
    type           TEXT NOT NULL CHECK (type IN ('person','project','tool','concept','decision','preference', 'other')),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_entities_name_type
ON entities(canonical_name, type);

CREATE TABLE entity_aliases (
    entity_id        TEXT NOT NULL REFERENCES entities(id),
    alias            TEXT NOT NULL,
    source_episode_id TEXT REFERENCES episodes(id),
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (entity_id, alias)
);

-- FTS for deduplication candidate lookup
CREATE VIRTUAL TABLE entity_aliases_fts USING fts5(
    alias,
    content=entity_aliases
);

CREATE TABLE relations (
    id               TEXT PRIMARY KEY,
    subject_id       TEXT NOT NULL REFERENCES entities(id),
    predicate        TEXT NOT NULL,   -- e.g. "works_on", "decided", "prefers", "avoids"
    object_id        TEXT REFERENCES entities(id),    -- NULL if object is a literal
    object_literal   TEXT,            -- used when object is not an entity (e.g. "postgres")
    confidence       REAL NOT NULL DEFAULT 1.0,
    valid_from       DATETIME NOT NULL,
    valid_until      DATETIME,        -- NULL = currently true
    source_episode_id TEXT REFERENCES episodes(id),
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CHECK (object_id IS NOT NULL OR object_literal IS NOT NULL)
);

CREATE INDEX idx_relations_subject    ON relations(subject_id, valid_until);
CREATE INDEX idx_relations_object     ON relations(object_id, valid_until);
CREATE INDEX idx_relations_predicate  ON relations(predicate, valid_until);


-- EPISODE <-> ENTITY JOIN

CREATE TABLE episode_entities (
    episode_id TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    entity_id  TEXT NOT NULL REFERENCES entities(id),
    PRIMARY KEY (episode_id, entity_id)
);


-- PRAXIS (procedural/behavioral patterns)

CREATE TABLE patterns (
    id                TEXT PRIMARY KEY,
    description       TEXT NOT NULL,
    embedding         BLOB,           -- float32 vector for semantic match
    confidence        REAL NOT NULL DEFAULT 0.5,
    observation_count INTEGER NOT NULL DEFAULT 1,
    first_observed_at DATETIME NOT NULL,
    last_observed_at  DATETIME NOT NULL,
    last_decayed_at   DATETIME,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE VIRTUAL TABLE patterns_fts USING fts5(
    description,
    content=patterns,
    content_rowid=rowid
);
