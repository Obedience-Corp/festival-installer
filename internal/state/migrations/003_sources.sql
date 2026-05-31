CREATE TABLE IF NOT EXISTS sources (
    name       TEXT PRIMARY KEY,
    url        TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    added_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sources_url ON sources(url);
