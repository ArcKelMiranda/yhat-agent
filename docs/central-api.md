# Cerebro YHat — Central Repository API Contract

**Version**: 1.2-F2 (stable for F2; F3 will extend without breaking F2)

**Implementation status** (2026-10-08):

- `LocalDirTransport` and `HTTPTransport` both implemented in
  `internal/share/transport.go` on `feat/cerebro-f2-http`. F0, F1,
  and the F2-LocalDirTransport work on `main` are verified. The
  F2-HTTPTransport is implemented, verifier-independent passes
  8 new HTTPTransport tests + 2 config round-trip tests + 4 main
  integration tests + 1 subprocess e2e against a mock F3 server
  on `:28083`. The Windows CGO-free build SHA-256 is
  `0bba2eb42c9018a87afd1f7e92f41c4025569a0350fd58426427f82647b0e967`.
  **The F2-HTTPTransport is NOT yet committed to `main`**: the
  parent paused before committing because 3 tests in
  `internal/bandeja` are red since `c54642c` (F2-LocalDirTransport
  was merged). See "Known F1-D bug" below.
- F3 server lives at `/tmp/yhat-central` (separate repo, branch
  `master`, commit `ea5481f`). Not yet pushed to GitHub; awaiting
  the user to create `github.com/ArcKelMiranda/yhat-central`.

This document describes the wire contract between the yhat-agent client and the
central repository.

## Transports

The yhat-agent client supports two Transport implementations that satisfy the
shared `Transport` interface (`Send`, `Pull`, `ApplyRejection`):

| Transport              | Use case              | Config fields used                     |
|------------------------|-----------------------|---------------------------------------|
| `LocalDirTransport`     | Local dev / legacy    | `central_repo` (file path)            |
| `HTTPTransport`         | F2 → F3 production   | `central_url` + `central_token`        |

The dispatcher in `cmd/yhat-agent/sync.go` selects the transport by URL scheme:
`http://` or `https://` → `HTTPTransport`; any other value → `LocalDirTransport`.

The `central_url` and `central_token` fields are written to `config.yaml` by
`yhat-agent install` when `YHAT_CENTRAL_URL` is set in the environment. The token
is sourced from `YHAT_CENTRAL_TOKEN` if present; otherwise the operator edits
`config.yaml` manually and replaces the `change-me` placeholder.

### HTTP Transport (F2 client)

`HTTPTransport` speaks HTTP/1.1 with the following characteristics:

- **Auth**: Bearer token sent as `Authorization: Bearer <token>` on every request.
  The token is per-operator and stored in `config.yaml::central_token`.
- **Timeout**: 15 seconds per request (configurable via the `http.Client`).
- **Base URL stripping**: trailing slashes are stripped from `central_url` before
  joining with the path components below.

### LocalDirTransport (F2 file-based simulation)

The central repository is a directory tree. All files are UTF-8 JSON.

---

## Known F1-D bug blocking the F2-HTTPTransport commit

The F1-D HTTP bandeja server (`internal/bandeja`) has a regression
where 3 tests fail since `c54642c` (F2-LocalDirTransport merge):

- `TestRED24_EditNonExistentMemory` expects 500, gets 405.
- `TestRED25_RejectNonExistentMemory` expects 500, gets 405.
- `TestRED26_RejectEmptyReasonAllowed` expects JSON `ok=true`,
  decodes malformed JSON (the response is the default 405
  page "method not allowed", not the API JSON).

The handlers `handleEdit` and `handleReject` were registered with
the path without trailing slash, e.g.
`mux.HandleFunc("/api/edit", ...)`. In Go 1.22+, `http.ServeMux`
treats `/api/edit` and `/api/edit/` as different patterns. The
`doReq` test helper passes the path without trailing slash. The
mux matches `/` (the catch-all) for that request, which only
handles GET. For POST, the catch-all returns 404. The first fix
attempt in `feat/cerebro-f2-http` registered both `/api/edit`
and `/api/edit/` and added a `withMethod("POST", ...)` middleware
to centralize the method check. The 405 response persisted,
suggesting the mux is still matching the catch-all `/` handler.
The fix is small and isolated: move the
`r.Method != http.MethodPost` check out of each handler body
and rely on the central `withMethod` middleware. Tracked in
`odd/tasks/cerebro-f1.md` under "F1-D known bug". The F2-HTTPTransport
is paused on `feat/cerebro-f2-http` until this is fixed, so the
F1-D regression does not leak into `main`.

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
`--central-repo` flag on the `sync` command. When `central_url` is set (HTTP
transport), `central_repo` is left empty and the HTTP endpoints below are used.

---

## HTTP Endpoints (F2 HTTP Transport)

| Method | Path                | Description                                         |
|--------|---------------------|---------------------------------------------------|
| POST   | `/api/memories`     | Upload a local memory (→ 201 Created, 409 Conflict) |
| GET    | `/api/team`         | Pull team-approved memories (→ 200 OK)              |
| POST   | `/api/rejections`   | F3 only; F2 stub logs and returns nil             |

### POST /api/memories

Uploads a single local memory. Request body is `LocalPayload` JSON.
Returns 201 Created on success, 409 Conflict if the memory ID is already known.
Any non-2xx status is treated as a sync failure and recorded as `last_error`.

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

### GET /api/team

Returns a JSON array of team-approved memories. Response is `TeamPayload[]`.
The cursor query parameter (`?cursor=<id>`) is reserved for F3 pagination.
F2 clients send no cursor; F3 servers return a server-managed cursor.

```json
[
  {
    "id":             "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "origin":         "team",
    "type":           "rule",
    "title":          "Regla del equipo",
    "content":        "Contenido aprobado por el equipo.",
    "context":        "",
    "author":         "team",
    "created_at":     "2024-01-02T00:00:00Z",
    "team_status":     "approved",
    "team_updated_at": "2024-01-03T00:00:00Z"
  }
]
```

---

## File-Based Endpoints (LocalDirTransport)

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
| `id`        | string   | Memory UUID (matches file name for LocalDir; server-assigned for HTTP) |
| `origin`    | string   | Always `"local"` for outgoing memories          |
| `type`      | string   | `decision`, `rule`, `anomaly`, `improvement`   |
| `title`     | string   | 1–200 characters                               |
| `content`   | string   | 1–10000 characters                             |
| `context`   | string?  | Optional context; absent if empty              |
| `author`    | string   | Operator name from `config.yaml`               |
| `created_at`| string   | RFC3339 UTC timestamp of original creation      |

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

| Feature        | F2 LocalDirTransport    | F2 HTTPTransport                   | F3 (future)                       |
|----------------|-------------------------|------------------------------------|-------------------------------------|
| Transport      | File-based (file://)     | HTTP with bearer auth              | HTTP with roster auth              |
| Team approval  | N/A (F3 server)        | N/A (F3 server)                   | 3-of-5 quorum, roster from config |
| Rejection      | Contract documented      | Stub: logs Spanish, returns nil     | Team reason captured at vote time  |
| Retired        | N/A                     | N/A                                | `team_status: retired` + bandeja   |
| Cursor         | File-name cursor        | Empty (F3 will add server cursor) | Opaque server cursor (F3 TBD)    |
