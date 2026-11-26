-- A run records which provider profile it used, so a resume after a restart
-- resolves the same adapter and credential (story 7, ticket review P2).
ALTER TABLE runs ADD COLUMN provider_id TEXT NOT NULL DEFAULT '';

-- Files explicitly attached to a run; tool scope is the project root plus
-- these paths (story 16/17). Stored as deterministic JSON.
ALTER TABLE runs ADD COLUMN attachments_json TEXT NOT NULL DEFAULT '[]';
