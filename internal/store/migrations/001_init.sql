-- 001_init.sql: initial schema for Cerebro YHat v1.2
-- Applied on first open by the embedded migration runner.
-- Fails loudly if schema_version is missing or higher than CurrentSchemaVersion.
-- WAL mode and foreign_keys are applied via PRAGMA on every open (store.go).

CREATE TABLE IF NOT EXISTS memories (
    id              TEXT PRIMARY KEY,
    origin          TEXT NOT NULL CHECK (origin IN ('local','team')),
    type            TEXT NOT NULL CHECK (type IN ('decision','rule','anomaly','improvement')),
    title           TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    content         TEXT NOT NULL CHECK (length(content) BETWEEN 1 AND 10000),
    context         TEXT,
    content_hash    TEXT NOT NULL,
    author          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'proposed'
                    CHECK (status IN ('proposed','validated','rejected','archived')),
    validated_by    TEXT,
    validated_at    TIMESTAMP,
    reject_reason   TEXT,
    share_status    TEXT NOT NULL DEFAULT 'none'
                    CHECK (share_status IN ('none','queued','sent','accepted','rejected')),
    shared_at       TIMESTAMP,
    remote_id       TEXT UNIQUE,
    team_status     TEXT CHECK (team_status IN ('approved','retired')),
    team_updated_at TIMESTAMP,
    is_example      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status <> 'validated' OR (validated_by IS NOT NULL AND validated_at IS NOT NULL)),
    CHECK (share_status = 'none' OR status IN ('validated','archived')),
    CHECK (origin = 'local' OR remote_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_memories_origin_status ON memories(origin, status, type);

-- Unique index for BR7: no duplicate content_hash for proposed or validated rows.
-- SQLite partial unique indexes are supported via the WHERE clause.
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_live_hash
    ON memories(origin, content_hash) WHERE status IN ('proposed','validated');

CREATE TABLE IF NOT EXISTS upload_queue (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id     TEXT NOT NULL REFERENCES memories(id),
    ordered_by    TEXT NOT NULL,
    ordered_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    attempts      INTEGER NOT NULL DEFAULT 0,
    last_error    TEXT,
    next_retry_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sync_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
