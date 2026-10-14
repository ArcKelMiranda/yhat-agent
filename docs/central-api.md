# Cerebro YHat — Central Repository API Contract

**Version**: 1.2-F2 (stable for F2; F3 will extend without breaking F2)

This document describes the wire contract between the yhat-agent client and the
central repository (implemented as F3 server in a future iteration, or as the
`LocalDirTransport` file-based simulation in F2).

The central repository is a directory tree. All files are UTF-8 JSON.

---

## Directory Layout

```
<central_repo>/
  <memory_id>.json        — outgoing local memories (written by yhat-agent)
  team/
    <memory_id>.json      — team-approved memories (read by yhat-agent on pull)
  rejected/
    <memory_id>.json      — team rejections (written by yhat-agent on rejection)
```

`<central_repo>` is read from `config.yaml` (`central_repo` field) or set via
`--central-repo` flag on the `sync` command. In F2, it must be a local directory
path (no HTTP transport yet).

---

## Outgoing Local Memory (`<memory_id>.json`)

Written by yhat-agent when `share.Sync` drains the upload queue and the transport
succeeds. File name is the memory's UUID (without extension).

```json
{
  "id":         "550e8400-e29b-41d4-a716-446655440000",
  "origin":     "local",
  "type":       "decision",
  "title":      "Decisión de ejemplo",
  "content":    "Contenido de la memoria.",
  "context":    "Contexto opcional.",
  "author":     "operator-a",
  "created_at": "2024-01-01T00:00:00Z"
}
```

| Field       | Type     | Description                                    |
|-------------|----------|------------------------------------------------|
| `id`        | string   | Memory UUID (matches file name)                  |
| `origin`    | string   | Always `"local"` for outgoing memories          |
| `type`      | string   | `decision`, `rule`, `anomaly`, `improvement`   |
| `title`     | string   | 1–200 characters                               |
| `content`   | string   | 1–10000 characters                             |
| `context`   | string?  | Optional context; absent if empty              |
| `author`    | string   | Operator name from `config.yaml`               |
| `created_at`| string   | RFC3339 UTC timestamp of original creation    |

---

## Team-Approved Memory (`team/<memory_id>.json`)

Read by yhat-agent when `share.Sync` pulls from the central repository.
File name is the server-assigned remote ID (UUID without extension).

```json
{
  "id":             "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "origin":         "team",
  "type":           "rule",
  "title":          "Regla del equipo",
  "content":        "Contenido aprobado por el equipo.",
  "context":        "",
  "author":         "team",
  "created_at":     "2024-01-02T00:00:00Z",
  "team_status":    "approved",
  "team_updated_at": "2024-01-03T00:00:00Z"
}
```

| Field            | Type     | Description                                       |
|------------------|----------|---------------------------------------------------|
| `id`             | string   | Server-assigned remote ID (= file name without ext) |
| `origin`         | string   | Always `"team"` for team memories                 |
| `type`           | string   | As above                                          |
| `title`          | string   | As above                                          |
| `content`        | string   | As above                                          |
| `context`        | string?  | Optional context; absent if empty                 |
| `author`         | string   | Original author or `"team"`                       |
| `created_at`     | string   | RFC3339 UTC timestamp of original creation        |
| `team_status`     | string   | Currently only `"approved"`; `"retired"` is F3    |
| `team_updated_at` | string   | RFC3339 UTC timestamp of team approval             |

---

## Team Rejection (`rejected/<memory_id>.json`)

Written by yhat-agent when `share.Sync` applies a team rejection locally
(F3 protocol; F2 documents the contract for F3 to implement).

```json
{
  "memory_id":   "550e8400-e29b-41d4-a716-446655440000",
  "reason":       "Duplicate of existing decision.",
  "rejected_at": "2024-01-05T00:00:00Z"
}
```

| Field       | Type   | Description                         |
|-------------|--------|-------------------------------------|
| `memory_id`| string | Original local memory ID             |
| `reason`   | string | Human-readable rejection reason      |
| `rejected_at`| string | RFC3339 UTC rejection timestamp   |

---

## Sync State Cursor

The pull cursor is stored in `sync_state` under key `pull_cursor`.
It is the file name (without `.json`) of the last processed `team/` entry.
Empty cursor means start from the beginning.

```sql
-- Cursor persisted in sync_state table
UPSERT INTO sync_state (key, value) VALUES ('pull_cursor', '<last_processed_id>');
```

---

## Upload Queue Lifecycle

```
memory: proposed → validated → queued → sent → accepted
                            ↘ rejected
```

| State      | Description                                                |
|------------|------------------------------------------------------------|
| `proposed` | Newly captured memory, awaiting bandeja approval            |
| `validated`| Approved in bandeja; eligible for sharing                   |
| `queued`   | In upload_queue; will be sent on next sync               |
| `sent`     | Successfully sent to central repo; removed from queue     |
| `accepted` | Confirmed by central repo (F3 future)                      |
| `rejected` | Rejected by team (F3; surfaced in bandeja Enviados)      |

---

## F2 / F3 Extension Points

| Feature       | F2 (this document)   | F3 (future)                         |
|---------------|---------------------|--------------------------------------|
| Transport     | `LocalDirTransport` | HTTP transport with auth             |
| Team approval  | N/A (F3 server)     | 3-of-5 quorum, roster from config   |
| Rejection     | Contract documented | Team reason captured at vote time    |
| Retired       | N/A                 | `team_status: retired` + bandeja    |
| Cursor        | File-name cursor    | Opaque server cursor (F3 design TBD)|
