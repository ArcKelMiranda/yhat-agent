# Cerebro YHat v1.2 — F1 (capture + inbox)

## Intent
Real capture and approval flow. Replaces the F0 synthetic search
with a persistent SQLite store, three real MCP tools, a local
bandeja for human approval, and a rewritten `yhat-agent install`
that writes `config.yaml` from the operator's Windows identity.

## User constraints
- Branch `feat/cerebro-f1` from `main`; no commits to `main`
  until the user pushes them.
- No real WorkSpace writes; tests use `t.TempDir()` and the
  home override hook.
- No actual F2 or F3 work; F1 is fully self-contained and the
  store and bandeja are the only surfaces.
- One delegated writer at a time; one independent verifier
  per sub-task; native review started only after F1 is
  functionally complete and the candidate is clean.
- Commit per sub-task. Conventional Commits.

## Sub-tasks (forecast 5-6 days)

### F1-A. Persistent SQLite store (~1.5 days)
- [x] F1-A committed as `32a4380` on `feat/cerebro-f1` (1349 insertions,
  1 deletion, 4 files; see git log for the canonical BR-rule map
  and Windows cross-build SHA-256).
- `internal/store/store.go` with `Open(path) (*Store, error)` and
  a `Memories`, `UploadQueue`, `SyncState` typed surface.
- `internal/store/migrations/001_init.sql` matching the PRD
  schema exactly (`memories`, `upload_queue`, `sync_state`).
- Embedded migration runner that applies 001 on first open and
  fails loudly on a missing or future schema.
- `BR1` (default status proposed), `BR3` (validated_by/validated_at
  are non-null together), `BR5` (title 1-200, content 1-10000),
  `BR7` (unique-by-`content_hash` for `proposed` and `validated`),
  `BR9` (`share_status` only with status in `validated`/`archived`),
  `BR11` (originate from team never editable here).
- `BR13` sensitive-content filter, configurable per operator via
  the existing `.sensitive-content-patterns` file when present,
  with sensible default literals (AWS keys, generic
  bearer-style tokens, `BEGIN ... PRIVATE KEY` blocks). Documented
  in store.go as a contract the F1-B tool layer must satisfy
  before calling ProposeMemory. Implementation is F1-E.
- Test-driven: write the schema test first, then the migration,
  then the typed surface, then the BR rules.
- Notes for follow-ups: ProposeMemory does a full-table scan per
  insert for BR7; F1-B will switch to the partial unique index
  for the hot path. ContentHash uses strings.Clone as an NFC
  approximation; full Unicode NFC needs `golang.org/x/text/unicode/norm`
  if operators start writing non-ASCII content beyond plain accented
  Spanish.

### F1-B. Real MCP tools (~1.5 days)
- [ ] F1-B implementation in progress; authorization widened to
  include `internal/store/queries.go` and `internal/store/store.go`
  for FTS5 helpers and accessor methods only. The store package
  remains the source of truth for SQL; the MCP tool layer
  consumes typed methods and never reaches into private fields.
- `propose_memory` accepts `{type, title, content, context?}` and
  returns the saved record or the BR7/BR13/BL-side rejection
  detail. Sensitive-content matches return `IsError: true` with
  an actionable Spanish message; the operator never sees raw
  matches.
- `list_pending` returns the operator's `proposed` memories,
  oldest first, paginated at 50.
- `get_memory` accepts a UUID, returns the full record with
  `origin` (`local` or `team`) and audit metadata. F2 not
  implemented yet, so `team` records return an empty set today.
- `search_brain` keeps the F0 contract but reads the real
  memories table; falls back to the F0 synthetic fixtures only
  when the DB is empty so existing operator tests still pass.
- Test-driven with RED/GREEN: each tool first via the
  `mcp.NewInMemoryTransports` test harness, then via the
  real SDK `CommandTransport` against a built binary in a
  temp directory.

### F1-C. CLI install rewrite (~0.5 day)
- New `yhat-agent install` writes:
  - `%USERPROFILE%\.yhat\bin\yhat-agent.exe` symlink or copy
    (not the F0 symlink in PATH; the F1 layout moves the
    binary inside `.yhat`).
  - `%USERPROFILE%\.yhat\config.yaml` with `operator:` from
    `%USERNAME%` and the central repo URL placeholder.
  - `%USERPROFILE%\.yhat\state.json` with version, registered
    apps, schema version, last sync.
- Existing OpenCode + packaged Claude registration kept
  intact (no regression on what F0 already proved).
- `yhat-agent status` extended to show DB path, schema version,
  pending count, last sync.
- `yhat-agent bandeja` opens `127.0.0.1:<port>/?token=<one-shot>`
  via `start`/`xdg-open`/`open` and exits after the page
  loads; the HTTP server keeps running in the background.

### F1-D. Bandeja HTTP server (~2 days)
- `internal/bandeja/bandeja.go` exposing `Start(ctx, store) (*Server, error)`
  that listens on `127.0.0.1:0` and mints a one-shot token.
- Endpoints:
  - `GET /?token=...` — the page (Aprobar/Rechazar/Editar,
    Enviados section).
  - `POST /api/approve?token=...&id=...` — records validation.
  - `POST /POST /api/reject?token=...&id=...&reason=...` —
    records rejection with a free-text reason (min 1 char,
    max 500).
  - `POST /api/edit?token=...&id=...&title=...&content=...` —
    updates title and content; the F1 store keeps `origin`,
    `status`, and history intact.
- Embedded HTML/CSS in `internal/bandeja/page.go` with simple
  Spanish copy and no JS framework. Reachable from the bandeja
  page only via the one-shot token; a request without a valid
  token returns 404.
- Auto-shutdown after 15 minutes idle (`time.Since(lastRequest) > 15m`).
- Test-driven: spawn a real listener in a test, drive the
  endpoints with `net/http`, assert the store mutated exactly
  the intended fields.

### F1-E. Sensitive-content filter (~0.5 day)
- `internal/sensitive/sensitive.go` with `Scan(text string) []Match`
  and a default literal set; optional local pattern file
  reuse of the existing `.sensitive-content-patterns` location.
- Test-driven with hand-crafted corpora: AWS keys, GitHub
  PATs, PEM blocks, basic username/host patterns.
- `propose_memory` runs the filter on `title + content +
  context` and rejects when any literal matches.

### F1-F. WorkSpace regression run (~0.5 day)
- The previous F0 install wrote the old `~/.config/opencode`
  layout; F1 must not break that, just coexist.
- After F1 lands, the user reruns the WorkSpace validation
  (operator identity, OpenCode registry, Claude packaged
  registry, fts5_search still works, propose_memory + bandeja
  create + approve + reject paths).

## Acceptance and checks

- All F1 tests pass locally with the direct compiler.
- `go vet ./...` clean.
- `yhat-agent mcp --selftest` keeps passing (backward
  compatibility of the F0 fixture fallback).
- A test using `t.TempDir()` exercises: install writes
  `config.yaml` with the operator's name, propose_memory
  creates a row, list_pending returns it, the bandeja approves
  it, get_memory returns `status: validated`.
- BR rules covered by named tests:
  - BR1 default proposed
  - BR3 validated_by/at non-null together
  - BR5 length bounds
  - BR7 unique-by-`content_hash`
  - BR9 share_status only with validated/archived
  - BR11 team records not editable locally
  - BR13 sensitive-content filter blocks real-looking AWS
    and GitHub keys
- Real subprocess test: build the binary into a temp dir,
  start the MCP server, run a happy path, then a sensitive
  rejection, then a duplicate-by-hash rejection, then a
  bandeja approve flow.

## Delivery

- One feature branch `feat/cerebro-f1` per sub-task; merge
  to `main` only after the user authorizes.
- Forecast: 5-6 days of focused work, not one session.
- Strategy ask-on-risk before any commit or PR over 400
  authored lines.
- Native review START only after F1-F is fully green and
  the user gives the go-ahead.
- No push without explicit user consent.

## Out of scope

- F2 share, upload queue drain, sync, central API.
- F3 server, quorum, roster.
- Power BI, semantic search, update mechanism.
- Migration of any data from the F0 store (the F0 store
  has no persistent file; nothing to migrate).
