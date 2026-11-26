-- Provider profiles (TAD §13.3). secret_ref is an OS-keychain reference;
-- raw API keys must never be written to this database.
CREATE TABLE provider_profiles (
    id           TEXT PRIMARY KEY,
    provider_id  TEXT NOT NULL,
    display_name TEXT NOT NULL,
    base_url     TEXT NOT NULL,
    secret_ref   TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE INDEX idx_provider_profiles_kind ON provider_profiles(provider_id);
