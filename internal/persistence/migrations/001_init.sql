-- v1 core schema (TAD §13.3). Later tickets add their tables as new migrations.
CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    root_uri    TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE conversations (
    id                 TEXT PRIMARY KEY,
    project_id         TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title              TEXT NOT NULL,
    active_harness_id  TEXT NOT NULL DEFAULT 'chat',
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);
CREATE INDEX idx_conversations_project_updated
    ON conversations(project_id, updated_at DESC, id DESC);

CREATE TABLE messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,
    content_json    TEXT NOT NULL,
    created_at      TEXT NOT NULL
);
CREATE INDEX idx_messages_conversation ON messages(conversation_id);

CREATE TABLE runs (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    status          TEXT NOT NULL,
    model_ref       TEXT NOT NULL,
    harness_id      TEXT NOT NULL,
    started_at      TEXT NOT NULL,
    finished_at     TEXT
);
CREATE INDEX idx_runs_conversation ON runs(conversation_id);

CREATE TABLE run_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id       TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    seq          INTEGER NOT NULL,
    event_type   TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    UNIQUE(run_id, seq)
);
CREATE INDEX idx_run_events_run_seq ON run_events(run_id, seq);

CREATE TABLE usage_records (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id             TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    provider_id        TEXT NOT NULL,
    model_id           TEXT NOT NULL,
    input_tokens       INTEGER NOT NULL DEFAULT 0,
    output_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    uncached_tokens    INTEGER NOT NULL DEFAULT 0,
    estimated          INTEGER NOT NULL DEFAULT 0,
    cost_estimate      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_usage_run ON usage_records(run_id);
