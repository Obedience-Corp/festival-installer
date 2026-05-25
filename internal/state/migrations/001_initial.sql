CREATE TABLE IF NOT EXISTS receipts (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    package_name    TEXT    NOT NULL,
    package_version TEXT    NOT NULL,
    installed_at    TEXT    NOT NULL,
    UNIQUE(package_name, package_version)
);

CREATE INDEX IF NOT EXISTS idx_receipts_name ON receipts(package_name);

CREATE TABLE IF NOT EXISTS locks (
    name        TEXT PRIMARY KEY,
    holder      TEXT NOT NULL,
    acquired_at TEXT NOT NULL
);
