CREATE TABLE user (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    role          TEXT    NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'viewer')),
    created_at    TEXT    NOT NULL,
    updated_at    TEXT    NOT NULL,
    last_login_at TEXT
);

CREATE TABLE session (
    token_hash   TEXT    PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES user (id) ON DELETE CASCADE,
    created_at   TEXT    NOT NULL,
    expires_at   TEXT    NOT NULL,
    last_seen_at TEXT    NOT NULL
);

CREATE INDEX session_user ON session (user_id);
CREATE INDEX session_expires_at ON session (expires_at);

CREATE TABLE sync_run (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at  TEXT    NOT NULL,
    finished_at TEXT    NOT NULL,
    duration_ms INTEGER NOT NULL,
    mode        TEXT    NOT NULL,
    source      TEXT    NOT NULL CHECK (source IN ('startup', 'schedule', 'manual')),
    success     INTEGER NOT NULL,
    error       TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX sync_run_started_at ON sync_run (started_at);

CREATE TABLE sync_run_replica (
    run_id      INTEGER NOT NULL REFERENCES sync_run (id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    target      TEXT    NOT NULL,
    profile     TEXT    NOT NULL,
    mode        TEXT    NOT NULL,
    duration_ms INTEGER NOT NULL,
    error       TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (run_id, position)
);
