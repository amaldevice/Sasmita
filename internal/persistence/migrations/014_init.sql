-- A run records which provider profile it used, so a resume after a restart
-- resolves the same adapter and credential (story 7, ticket review P2).
ALTER TABLE runs ADD COLUMN provider_id TEXT NOT NULL DEFAULT '';
