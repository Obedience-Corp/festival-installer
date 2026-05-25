DROP INDEX IF EXISTS idx_receipts_name;
DROP TABLE IF EXISTS receipts;

CREATE TABLE receipts (
    package_id   TEXT PRIMARY KEY,
    version      TEXT NOT NULL,
    source       TEXT NOT NULL,
    channel      TEXT NOT NULL,
    installed_at TEXT NOT NULL
);

CREATE INDEX idx_receipts_source ON receipts(source);
CREATE INDEX idx_receipts_installed_at ON receipts(installed_at);

CREATE TABLE receipt_files (
    receipt_pkg_id TEXT NOT NULL,
    path           TEXT NOT NULL,
    hash           TEXT NOT NULL,
    mode           INTEGER NOT NULL,
    PRIMARY KEY (receipt_pkg_id, path),
    FOREIGN KEY (receipt_pkg_id) REFERENCES receipts(package_id) ON DELETE CASCADE
);

CREATE TABLE receipt_metadata (
    receipt_pkg_id TEXT NOT NULL,
    key            TEXT NOT NULL,
    value          TEXT NOT NULL,
    PRIMARY KEY (receipt_pkg_id, key),
    FOREIGN KEY (receipt_pkg_id) REFERENCES receipts(package_id) ON DELETE CASCADE
);
