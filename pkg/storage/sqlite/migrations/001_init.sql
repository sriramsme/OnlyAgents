CREATE TABLE IF NOT EXISTS conversations (
    id            TEXT PRIMARY KEY,
    channel       TEXT NOT NULL,        -- "telegram", "onlyagents", "discord"
    agent_id      TEXT NOT NULL,
    chat_id       TEXT NOT NULL,
    started_at    TEXT NOT NULL,
    ended_at      TEXT,
    context       TEXT NOT NULL DEFAULT '{}',
    summary       TEXT NOT NULL DEFAULT '',
    peer_agent_id TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_active_conversation
    ON conversations(channel, agent_id)
    WHERE ended_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_conversations_agent_time
    ON conversations(agent_id, started_at);

CREATE TABLE IF NOT EXISTS messages (
    id                TEXT PRIMARY KEY,
    conversation_id   TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    agent_id          TEXT NOT NULL,
    role              TEXT NOT NULL,              -- user | assistant | tool
    content           TEXT NOT NULL DEFAULT '',
    reasoning_content TEXT NOT NULL DEFAULT '',
    tool_calls        TEXT NOT NULL DEFAULT '[]', -- JSON []ToolCall, populated for role=assistant
    tool_call_id      TEXT NOT NULL DEFAULT '',   -- populated for role=tool
    timestamp         TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_conv
    ON messages (conversation_id, timestamp ASC);
CREATE INDEX IF NOT EXISTS idx_messages_agent_time
    ON messages (agent_id, timestamp ASC);
CREATE INDEX idx_messages_session_agent
    ON messages(conversation_id, agent_id, timestamp);

CREATE TABLE IF NOT EXISTS agent_state (
    agent_id                TEXT PRIMARY KEY,
    current_conversation_id TEXT NOT NULL DEFAULT '',
    context                 TEXT NOT NULL DEFAULT '{}',
    preferences             TEXT NOT NULL DEFAULT '{}',
    goals                   TEXT NOT NULL DEFAULT '[]',
    last_active             TEXT NOT NULL
);
