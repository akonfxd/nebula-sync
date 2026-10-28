CREATE TABLE profile (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    full_sync   INTEGER NOT NULL DEFAULT 0,
    run_gravity INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);

CREATE TABLE profile_gravity (
    profile_id          INTEGER PRIMARY KEY REFERENCES profile (id) ON DELETE CASCADE,
    dhcp_leases         INTEGER NOT NULL DEFAULT 0,
    "group"             INTEGER NOT NULL DEFAULT 0,
    adlist              INTEGER NOT NULL DEFAULT 0,
    adlist_by_group     INTEGER NOT NULL DEFAULT 0,
    domainlist          INTEGER NOT NULL DEFAULT 0,
    domainlist_by_group INTEGER NOT NULL DEFAULT 0,
    client              INTEGER NOT NULL DEFAULT 0,
    client_by_group     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE profile_section (
    profile_id  INTEGER NOT NULL REFERENCES profile (id) ON DELETE CASCADE,
    section     TEXT    NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0,
    filter_type TEXT    NOT NULL DEFAULT 'none' CHECK (filter_type IN ('none', 'include', 'exclude')),
    PRIMARY KEY (profile_id, section)
);

CREATE TABLE profile_key (
    profile_id INTEGER NOT NULL,
    section    TEXT    NOT NULL,
    key        TEXT    NOT NULL,
    position   INTEGER NOT NULL,
    PRIMARY KEY (profile_id, section, key),
    FOREIGN KEY (profile_id, section) REFERENCES profile_section (profile_id, section) ON DELETE CASCADE
);

CREATE TABLE pihole (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    role         TEXT    NOT NULL CHECK (role IN ('primary', 'replica')),
    name         TEXT    NOT NULL,
    url          TEXT    NOT NULL,
    password_enc TEXT    NOT NULL,
    enabled      INTEGER NOT NULL DEFAULT 1,
    profile_id   INTEGER REFERENCES profile (id) ON DELETE SET NULL,
    position     INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL
);

CREATE UNIQUE INDEX pihole_single_primary ON pihole (role) WHERE role = 'primary';

CREATE TABLE schedule (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    cron TEXT
);

CREATE TABLE webhook (
    outcome  TEXT    PRIMARY KEY CHECK (outcome IN ('success', 'failure')),
    url      TEXT    NOT NULL DEFAULT '',
    method   TEXT    NOT NULL DEFAULT 'POST',
    body_enc TEXT    NOT NULL DEFAULT '',
    skip_tls INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE webhook_header (
    outcome   TEXT NOT NULL REFERENCES webhook (outcome) ON DELETE CASCADE,
    key       TEXT NOT NULL,
    value_enc TEXT NOT NULL,
    PRIMARY KEY (outcome, key)
);
