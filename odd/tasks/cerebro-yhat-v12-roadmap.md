# Cerebro YHat v1.2 — implementation roadmap

## State at session end (2026-10-08, after 7-question round)

- `main` is at `2b9ebbc` and matches `origin/main`.
- Tag `v0.2.0-f0` points to `f4b9a61` (one commit behind, no rebuild).
- `go install @main` and `go install @v0.2.0-f0` work in WorkSpaces
  that have Go on PATH. Operators on the 5 WorkSpaces must use one
  of these forms, not bare SHAs.
- The diagnostic F0 prototype is verified end-to-end on the user
  WorkSpace: OpenCode registered, packaged Claude Desktop
  registered, `fts5_search` returns the expected fixture, and
  `mcp --selftest` passes.

## PRD open questions (all 7 answered, observation 411)

1. **Central repository** → **new dedicated service**. F3 is
   greenfield server work. yhat-knowledge is not the target.
2. **Sharing trigger** → **chat only**. No Enviar button in the
   bandeja. BR10 is satisfied by the chat instruction.
3. **Team approval** → **majority of 5 operators (3 or more)**. F3
   quorum is 3/5; F2 envelope includes per-operator vote state.
4. **Rejection visibility** → **visible to author with reason**.
   F2 sync payload carries `rejection_reason`; F1 bandeja Enviados
   shows it.
5. **Operator identity** → **Windows `%USERNAME%` automatically**.
   `yhat-agent install` writes it to `config.yaml` without prompts.
6. **`yhat-mcp-server` coexistence** → **coexist in parallel**.
   The new MCP entry is named `yhat`; no migration; both visible
   to the operator.
7. **Sync cadence** → **on Claude Desktop open + every 15 minutes**.
   Background scheduler; never blocks the user.

## F1 scope (capture + inbox), estimated 5-6 days

- `internal/store` with SQLite WAL, migrations under
  `internal/store/migrations/001_init.sql`, schema from the PRD
  (memories + upload_queue + sync_state).
- Persistent store at `%USERPROFILE%\.yhat\yhat.db` (Linux
  development uses `$HOME/.yhat/yhat.db`).
- MCP tools replacing the F0 diagnostic:
  - `propose_memory` (decision/rule/anomaly/improvement, 1-200 chars
    title, 1-10000 content, optional context, sensitive-content
    validation, duplicate-by-`content_hash` rejection).
  - `list_pending` (operator's unapproved memories, oldest first).
  - `get_memory` (full record, origin and history).
  - `search_brain` extended to read the real DB with a fallback
    to the synthetic F0 fixtures only when the DB is empty.
- `internal/bandeja` HTTP server on `127.0.0.1` with a random
  port, one-shot token in the URL, auto-shutdown after 15 min idle.
  Embedded HTML with Aprobar / Rechazar (con motivo) / Editar and
  an Enviados section showing rejections with their reason.
- `yhat-agent install` extended to write `config.yaml` (operator
  identity from `%USERNAME%`), `state.json` (installed version
  and registered apps) and the DB directory.
- `yhat-agent bandeja` CLI flag opens the local browser via
  `start` (Windows) / `xdg-open` / `open` (macOS).
- `yhat-agent status` shows DB path, schema version, pending
  count, last sync timestamp.
- BR1-BR13 enforced in `store/` and `bandeja/`, never as
  agent instructions.

## F2 scope (share + pull), estimated 4-5 days, after F1

- `share_memory` enqueues approved memories to `upload_queue` with
  the operator identity from `config.yaml` and the chat-side
  metadata that Claude passes.
- `sync` subcommand runs at Claude Desktop open and every 15
  minutes, with cursor in `sync_state` so partial failures do not
  re-upload already-synced memories.
- The API contract is documented against the new dedicated F3
  service (still greenfield; design here unlocks the F3 estimate).

## F3 scope (team approval, greenfield server)

- New repository, new codebase. Only the contract from F2 is
  inherited; the server has no client-side coupling.
- 3-of-5 quorum over a roster registered in `config.yaml` on
  the server side.
- Rejection reason captured at vote time; surfaced through the
  F2 sync envelope so the author's bandeja shows it.
- Estimate: not yet possible without the server tech choice.
  Tracked in `odd/tasks/cerebro-f3.md` once the user picks.

## F4 (Power BI) and F5 (semantic search)

Deferred until the 5 operators are actually using the capture
flow. Gating on real data, not on technology choices.

## Backlog items not yet scheduled

- `yhat-agent update` for in-place binary replacement on Windows,
  with rollback if `--selftest` after the swap fails.
- Migration path for the previous Engram-based assets already
  present in `assets/agents/yhat-memory-capture.md` and
  `assets/skills/yhat-memory-capture/SKILL.md`. The current
  `yhat-agent install` keeps writing them; the F1 store will
  consult the local SQLite first and only fall back to the
  embedded assets if the DB is empty.
- Documentation for the 5 non-developer operators: how to
  install the binary, how Claude Desktop picks it up, what the
  bandeja looks like.
- GitHub Actions release workflow that produces a Windows
  `.exe` plus a SHA-256 manifest and uploads it as a release
  asset.
- Conversion of the existing CLI install from
  `~/.config/opencode/...` to the new `%USERPROFILE%\.yhat\...`
  layout. The PRD's appendix already lays out the directory
  tree; F1 must migrate anything the F0 install put in place
  on the user WorkSpace.

## Status of native review

Lineage `review-a921ac9aaf9b4d74` ran all four lenses and
surfaced two candidate-caused findings (R3, R4). Corrections
applied. Final validation capture repeatedly rejected the
reoffered binding as stale; user disabled review mode at clone
scope. No review approval. A new session can re-enable and
retry, or the user can abandon the lineage and start a fresh
START for F1.
