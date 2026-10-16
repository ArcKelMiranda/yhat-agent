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
- [x] F1-D landed on `feat/cerebro-f1` as commit `226d1ee`
  (1908 insertions, 6 files). Windows build SHA-256
  `4b133394a53ccc1a518c42251d319df255bc77ec6669575b30b0b3f158188674`.
- New `internal/bandeja` package: `Server.Start/URL/Close/Wait`
  on 127.0.0.1:0, single 32-byte token, 404 without token,
  15-min idle shutdown, operator identity from
  `config.yaml` then `USERNAME` then `USER` then `unknown`.
- Routes: `GET /` returns Spanish HTML with Pendientes and
  Enviados sections. `POST /api/approve`, `POST /api/reject`
  (optional reason, max 500 chars), `POST /api/edit` (title
  1-200, content 1-10000). All POSTs re-validate the token.
- Embedded Spanish HTML/CSS/JS in `page.go`; HTML escaping
  for memory fields. No external assets, no framework.
- New `yhat-agent bandeja` subcommand starts the server in
  foreground. `yhat-agent install` also opens the bandeja at
  the end unless `--no-bandeja` is set. Cross-platform
  browser open (Windows: `start ""`; macOS: `open`; Linux:
  `xdg-open`); falls back gracefully if missing.
- New `store.ListByShareStatus` in `queries.go` for the
  Enviados section.
- 26 internal/bandeja tests + 3 bandeja integration tests.
- User chose this turn: subcommand + auto-open on install;
  reject reason optional; Enviados shows estado y razón.

#### F1-D bandeja bug (resolved)

**Resolved on 2026-10-16 in `feat/cerebro-f2-http`.** The earlier
note ("3 RED tests since `c54642c`") was wrong: 14 of 26 bandeja
tests failed since the original F1-D commit `226d1ee`, so F1 was
merged to `main` with Aprobar/Rechazar/Editar broken. Two bugs
combined:

1. Production: the mutation routes were registered only as
   `/api/approve/`, `/api/reject/`, `/api/edit/`, while the page JS
   posts to `/api/approve` (no slash). ServeMux answers with a 301,
   the browser follows it as a GET, and `withMethod("POST")` returns
   405 "method not allowed". Fix: register the routes without the
   trailing slash (the id travels in the JSON body).
2. Tests: the `baseURL` helper kept the trailing slash of
   `http://addr/?token=...`, so tests requested `//api/approve`,
   which ServeMux also redirects. Fix: trim the slash.

Both fixes are needed: with only the helper fixed, 17 tests still
fail. All 26 bandeja tests pass now.

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

## Final state of F1

- All five sub-tasks A through E landed on `feat/cerebro-f1`
  and merged into `main` as commit `b007ad2`. The merge is
  a `ort` strategy `--no-ff` with an explicit merge commit.
- Tag `v0.3.0-f1` points to the pre-merge F1 tip
  (`038011d`); F0 is still tagged as `v0.2.0-f0`.
- F1-F is the manual WorkSpace regression runbook and lives
  in `odd/tasks/cerebro-f1f.md`. It is not code; the user
  runs the 8 tests in the WorkSpace and reports back.
- The `go install` path for the user is
  `go install github.com/ArcKelMiranda/yhat-agent/cmd/yhat-agent@v0.3.0-f1`
  (or `@main` after the merge is in place; both resolve to
  the same binary now).

## Acceptance summary for the F1 sub-task set

- F0 baseline: `mcp --selftest` returns the expected
  Política Fixture Beta fixture on every commit. F0 OpenCode
  and packaged Claude Desktop registrations are preserved
  untouched.
- BR1, BR3, BR5, BR7, BR9, BR11: enforced in
  `internal/store/store.go`, with named tests in
  `internal/store/store_test.go`.
- BR13: enforced in `internal/sensitive/sensitive.go`,
  called from `cmd/yhat-agent/mcp.go` before
  `store.ProposeMemory`. Masking in the model-facing
  response uses `4+****+4` (max 12 visible chars, 40-byte
  cap).
- F1 tools: `propose_memory`, `list_pending`, `get_memory`,
  `search_brain` registered in `cmd/yhat-agent/mcp.go` and
  covered by 17+ mcp tests.
- F1-C install: writes `config.yaml` (with `%USERNAME` on
  Windows), `state.json`, and creates the SQLite DB at
  `%USERPROFILE%\.yhat\yhat.db`. Operator identity precedence
  documented in code.
- F1-D bandeja: HTTP server on `127.0.0.1:0` with one-shot
  32-byte token, Spanish HTML page with Pendientes and
  Enviados sections, 15 min idle shutdown. 26 bandeja tests
  + 3 integration tests. Operator can disable the
  auto-open with `--no-bandeja`.

## Out of scope

- F2 share, upload queue drain, sync, central API.
- F3 server, quorum, roster.
- Power BI, semantic search, update mechanism.
- Migration of any data from the F0 store (the F0 store
  has no persistent file; nothing to migrate).

---

# Cerebro YHat v1.2 — F2-cliente HTTPTransport

## Status
- Branch: `feat/cerebro-f2-http` from `feat/cerebro-f2` (commit
  `c54642c feat(share): F2 share_memory, sync, and central-api contract`).
- Implementation: done. Verifier passed. 8 new HTTPTransport tests +
  2 config round-trip tests + 4 main integration tests + 1 subprocess
  e2e against a mock F3 server on `:28083`.
- Windows CGO-free build: SHA-256
  `0bba2eb42c9018a87afd1f7e92f41c4025569a0350fd58426427f82647b0e967`.
- F0 baseline: `mcp --selftest` still returns Política Fixture Beta.
- **Status: committed** on `feat/cerebro-f2-http` together with
  the F1-D fix (see "F1-D bandeja bug (resolved)" above) and a
  rewrite of the global `--central-token` parsing: the previous
  `flag.FlagSet` parsed subcommand args (noise on
  `mcp --selftest`, token ignored after other flags, stray value
  left behind with `--central-token X`). `extractCentralToken` now
  removes the flag from any position, before or after the
  subcommand.
- Windows CGO-free build (`-trimpath`) after these fixes: SHA-256
  `722f1b0fee7ba784f1ee74c9a45591147339c877c9c7651d56dd07c89d1c246b`.

## User constraints
- No commits to `main` until the user pushes them.
- No change to the F3 server (lives at `/tmp/yhat-central`).
- No change to F0, F1, MSI.

## Implementation summary

### Config fields (`internal/config/config.go`)
- `Config.CentralURL string` — F2 HTTP transport base URL
  (e.g. `https://yhat-central.example.local:8080`). YAML tag
  `central_url`. Round-trips through Save/Load.
- `Config.CentralToken string` — Bearer token per operator. YAML
  tag `central_token`. Round-trips through Save/Load.

### HTTPTransport (`internal/share/transport.go`)
- `NewHTTPTransport(client *http.Client, baseURL, token)` — constructor.
  Strips trailing slash from baseURL.
- `HTTPTransport.Send` → `POST /api/memories` with `LocalPayload` JSON.
  Returns nil on 2xx; propagates non-2xx as
  `fmt.Errorf("HTTPTransport.Send: server returned %s", resp.Status)`.
- `HTTPTransport.Pull` → `GET /api/team` returning `[]TeamPayload`.
  `NextCursor` is empty for F2 (F3 will add server-side cursor).
- `HTTPTransport.ApplyRejection` — F2 stub. Logs a Spanish comment and
  returns nil. F3 will implement `POST /api/rejections`.
- `NewTransport(url, token)` — factory: `http://`/`https://` →
  `HTTPTransport`; anything else → `LocalDirTransport`.
- 15-second timeout per request (set on the `http.Client`).
- Bearer token sent as `Authorization: Bearer <token>` on every request.

### Config write (`cmd/yhat-agent/main.go` — `runInstallF1`)
- When `YHAT_CENTRAL_URL` env var is set → writes `central_url` and
  `central_token` (from `YHAT_CENTRAL_TOKEN` if set, else `"change-me"`
  placeholder).
- When `YHAT_CENTRAL_URL` is not set → `central_repo` is set to the
  placeholder, `central_url` and `central_token` are empty.

### CLI flags (`cmd/yhat-agent/main.go`)
- `--central-token=<token>` — global flag parsed before subcommand routing.
  Overrides `config.CentralToken` at runtime.
- `--central-url=<url>` on `sync` — overrides both `central_url` and
  `central_repo` from config.
- `--central-repo=<path>` on `sync` — preserved for backward compat;
  overrides `central_repo` from config.

### Sync dispatcher (`cmd/yhat-agent/sync.go`)
- `runBackgroundSyncOnce` and `runSyncCommand` both use `share.NewTransport`
  (URL-scheme-based factory) with the resolved URL and token.
- Resolution order: CLI flags (`--central-url` > `--central-repo`) >
  config (`central_url` > `central_repo`) > placeholder.

### Tests
- `internal/share/transport_test.go`: 8 new HTTPTransport tests covering
  happy path, 409 duplicate, Pull empty list, Pull team approvals, timeout,
  bearer header verification, JSON shape validation, and ApplyRejection stub.
- `internal/config/config_test.go`: 2 new round-trip tests for
  `CentralURL`/`CentralToken`.
- `cmd/yhat-agent/main_test.go`: 4 new tests for install with env vars,
  `getCentralToken` CLI override, and `getCentralToken` fallback.
- `cmd/yhat-agent/sync_test.go`: `TestHTTPTransport_E2E_Subprocess` —
  subprocess test that starts the mock F3 server binary at
  `/tmp/yhat-central-bin/yhat-central` on `:28083`, sends a memory via
  `HTTPTransport.Send`, casts 3 votes via the F3 HTTP API, then pulls
  `/api/team` and asserts the memory is approved.

### End-to-end verification
- `CGO_ENABLED=0 go test -count=1 ./internal/share` — all 15 tests pass.
- `CGO_ENABLED=0 go test -count=1 ./internal/config` — all 9 tests pass.
- `CGO_ENABLED=0 go test -count=1 ./cmd/yhat-agent` — all 18 tests pass.
- `CGO_ENABLED=0 go vet ./...` — clean.
- `yhat-agent mcp --selftest` — F0 Política Fixture Beta passes.

## Out of scope

- Changes to the F3 server (separate repo at `/tmp/yhat-central`).
- F4/F5 features.
- TLS termination, mTLS.
- Bulk migration tooling from LocalDirTransport to HTTPTransport.
- Per-operator team approval (handled by F3 quorum).
