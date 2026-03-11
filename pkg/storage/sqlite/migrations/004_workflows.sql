CREATE TABLE workflows (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    created_by TEXT NOT NULL,
    status TEXT NOT NULL,
    channel_json TEXT NOT NULL,
    original_message TEXT NOT NULL,
    metadata TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE wf_tasks (
    id TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    type TEXT NOT NULL,
    channel_json TEXT NOT NULL,
    depends_on TEXT,
    required_capabilities TEXT,
    payload TEXT,
    status TEXT NOT NULL,
    result TEXT,
    error TEXT,
    assigned_agent_id TEXT,
    created_at TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    timeout_seconds INTEGER,
    metadata TEXT,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE INDEX idx_wf_tasks_workflow ON wf_tasks(workflow_id);
CREATE INDEX idx_wf_tasks_status ON wf_tasks(status);
CREATE INDEX idx_wf_tasks_created ON wf_tasks(created_at);
