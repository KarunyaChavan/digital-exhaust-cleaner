CREATE TABLE IF NOT EXISTS scan_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    root_path TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE TABLE IF NOT EXISTS directories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    scan_session_id INTEGER NOT NULL,
    UNIQUE(scan_session_id, path),
    FOREIGN KEY (scan_session_id) REFERENCES scan_sessions(id)
);

CREATE TABLE IF NOT EXISTS files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_session_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    directory_path TEXT NOT NULL,
    name TEXT NOT NULL,
    extension TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    created_at TEXT,
    modified_at TEXT NOT NULL,
    accessed_at TEXT NOT NULL,
    path_entropy REAL NOT NULL,
    sha256 TEXT,
    partial_sha256 TEXT,
    is_hidden INTEGER NOT NULL,
    is_symlink INTEGER NOT NULL,
    UNIQUE(scan_session_id, path),
    FOREIGN KEY (scan_session_id) REFERENCES scan_sessions(id)
);

CREATE INDEX IF NOT EXISTS idx_files_size ON files(size_bytes);
CREATE INDEX IF NOT EXISTS idx_files_sha256 ON files(sha256);
CREATE INDEX IF NOT EXISTS idx_files_accessed ON files(accessed_at);
CREATE INDEX IF NOT EXISTS idx_files_extension ON files(extension);
