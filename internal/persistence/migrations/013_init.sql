-- Tier 2 sandboxed artifacts (TAD §13.3, ticket #28). Payloads live on disk;
-- this table is the library index and carries the versioned runtime contract.
-- No foreign keys: migration lanes may land in any order.
CREATE TABLE IF NOT EXISTS artifacts (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL DEFAULT '',
    kind            TEXT NOT NULL DEFAULT 'sandboxed',
    uri             TEXT NOT NULL,
    mime_type       TEXT NOT NULL DEFAULT 'text/html',
    size_bytes      INTEGER NOT NULL DEFAULT 0,
    sha256          TEXT NOT NULL DEFAULT '',
    runtime_version TEXT NOT NULL,
    source_kind     TEXT NOT NULL,
    manifest_json   TEXT NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'ok',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_artifacts_project ON artifacts(project_id, updated_at);

CREATE TABLE IF NOT EXISTS artifact_versions (
    id           TEXT PRIMARY KEY,
    artifact_id  TEXT NOT NULL,
    version      INTEGER NOT NULL,
    uri          TEXT NOT NULL,
    source_ref   TEXT NOT NULL,
    compiled_ref TEXT NOT NULL,
    sha256       TEXT NOT NULL,
    size_bytes   INTEGER NOT NULL,
    created_at   TEXT NOT NULL,
    UNIQUE(artifact_id, version)
);
CREATE INDEX IF NOT EXISTS idx_artifact_versions_artifact ON artifact_versions(artifact_id, version);
