-- Ticket #23: approvals are bound to exact arguments and a policy snapshot;
-- checkpoints let a paused run resume after an application restart.
CREATE TABLE IF NOT EXISTS approvals (
    id             TEXT PRIMARY KEY,
    run_id         TEXT NOT NULL REFERENCES runs(id),
    conversation_id TEXT NOT NULL REFERENCES conversations(id),
    call_id        TEXT NOT NULL,
    tool_id        TEXT NOT NULL,
    mode           TEXT NOT NULL,
    project_id     TEXT NOT NULL,
    status         TEXT NOT NULL,
    decision       TEXT NOT NULL DEFAULT '',
    arguments_json TEXT NOT NULL,
    args_hash      TEXT NOT NULL,
    policy_hash    TEXT NOT NULL,
    action_text    TEXT NOT NULL,
    resource_text  TEXT NOT NULL,
    impact_text    TEXT NOT NULL,
    message        TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    resolved_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_approvals_run ON approvals(run_id, status);
CREATE INDEX IF NOT EXISTS idx_approvals_conversation ON approvals(conversation_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_approvals_binding ON approvals(run_id, call_id, args_hash, policy_hash);

CREATE TABLE IF NOT EXISTS policy_grants (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    tool_id       TEXT NOT NULL,
    resource_hash TEXT NOT NULL,
    policy_hash   TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    UNIQUE(project_id, tool_id, resource_hash, policy_hash)
);

CREATE TABLE IF NOT EXISTS checkpoints (
    id         TEXT PRIMARY KEY,
    state_blob BLOB NOT NULL,
    updated_at TEXT NOT NULL
);
