-- 002_widen_types.sql: memories.type grows from 4 to 12 values.
-- SQLite cannot alter a CHECK constraint, so the table is rebuilt. The runner
-- applies this inside a transaction with foreign_keys=OFF, copies rowid
-- explicitly (the FTS index is keyed by it) and recreates the table's triggers.

CREATE TABLE memories_new (
    id              TEXT PRIMARY KEY,
    origin          TEXT NOT NULL CHECK (origin IN ('local','team')),
    type            TEXT NOT NULL CHECK (type IN (
                        'decision','rule','anomaly','improvement',
                        'observation','assumption','process','mapping',
                        'exception','entity-definition','integration','technical-rule')),
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

INSERT INTO memories_new (rowid, id, origin, type, title, content, context, content_hash, author,
    status, validated_by, validated_at, reject_reason, share_status, shared_at, remote_id,
    team_status, team_updated_at, is_example, created_at, updated_at)
SELECT rowid, id, origin, type, title, content, context, content_hash, author,
    status, validated_by, validated_at, reject_reason, share_status, shared_at, remote_id,
    team_status, team_updated_at, is_example, created_at, updated_at
FROM memories;

DROP TABLE memories;
ALTER TABLE memories_new RENAME TO memories;

CREATE INDEX IF NOT EXISTS idx_memories_origin_status ON memories(origin, status, type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_live_hash
    ON memories(origin, content_hash) WHERE status IN ('proposed','validated');
