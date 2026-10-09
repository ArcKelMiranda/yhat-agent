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
- [x] F1-B landed on `feat/cerebro-f1` as commits `52df538`
  (tools + queries) and `2c17cf3` (dead-code cleanup and
  tightened subprocess DB check). Windows build SHA-256
  `f49493aff03b555b8ca713eb0805fe7ba674aacf2d56171903812f2e8c618a9a`.
- Authorization widened to include `internal/store/queries.go`
  and `internal/store/store.go` for FTS5 helpers and accessor
  methods only. The store package remains the source of truth
  for SQL; the MCP tool layer consumes typed methods and never
  reaches into private fields.
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
- [x] F1-C landed on `feat/cerebro-f1` as commit `7042b81`
  (882 insertions, 8 files; go.mod adds `gopkg.in/yaml.v3
  v3.0.1`). Windows build SHA-256
  `f3584895c73aab60cff0b78a97e7fdd19e9a006351f89272e5441e08b6a08769`.
- New `internal/config` package with typed `Config` and
  `State` structs, YAML config + JSON state, atomic write.
- `yhat-agent install` keeps the F0 OpenCode + Claude
  registration and now also writes `config.yaml`,
  `state.json` and creates the SQLite DB at
  `%USERPROFILE%\.yhat\yhat.db` (or platform equivalent).
  Operator identity from `%USERNAME` on Windows.
- `yhat-agent status` adds a `Cerebro` section with home,
  DB, schema, per-status counts, operator, and last sync.
  `--json` gains a `cerebro` key. Survives missing DB
  gracefully.
- 6 internal/config tests + 5 main integration tests.
- User chose this turn: keep the F0 binary location
  (don't move it), use a placeholder `central_repo` URL,
  show DB + counts + last sync in status.

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
- [x] F1-E landed on `feat/cerebro-f1` as commit `3c85c8a`
  (646 insertions, 4 files). Windows build SHA-256
  `3a9fd8b12cc95cdf7a441a27455fe942eb57bab0c228b876d277b8181455ed57`.
- Defaults-only policy (user chose it this turn). No
  `.sensitive-content-patterns` file is consulted by the F1
  filter; the legacy file remains untouched.
- Patterns: AKIA access key, GitHub PATs
  (gh[pousr]_[A-Za-z0-9]{20,}), PEM private key block,
  Bearer JWT, Slack `xox[baprs]-`, Anthropic `sk-ant-`,
  OpenAI `sk-`. Implemented as Go regexps with one-time
  init-time compilation.
- Masking in the model-facing response: at most 12 visible
  chars (4+4+4) and 40-byte cap, with **** in the middle.
- `propose_memory` calls `sensitive.Scan(title + \n + content
  + \n + context)`; on a non-empty match, returns IsError with
  Spanish copy, never calls `store.ProposeMemory`.
- 22 internal/sensitive tests + 2 new mcp tests cover the
  patterns, position arithmetic on UTF-8 text, multi-match,
  false-positive cases, and the no-persist guarantee.

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
