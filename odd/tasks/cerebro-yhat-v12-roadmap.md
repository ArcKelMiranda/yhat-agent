# Cerebro YHat v1.2 — implementation roadmap

## State at session end (2026-10-08)

- `main` is at `f4b9a61` and matches `origin/main`.
- Tag `v0.2.0-f0` points to `f4b9a61` and is published.
- `go install @main` and `go install @v0.2.0-f0` both work in WorkSpaces that
  have Go on PATH. `@ad0cf3a` (a bare SHA) requires `git` on PATH; use the
  main ref or a tag instead for the 5 non-developer operators.
- The diagnostic MCP server (synthetic FTS5 fixtures) is verified on the
  user WorkSpace: OpenCode registered, packaged Claude Desktop
  registered, search returns the expected fixture via `fts5_search`,
  status reports the assets as up to date.

## Open questions that block F1 (capture + inbox)

1. **Central repository.** Is it `yhat-knowledge` (existing UI on the
   server) or a brand new service? The F2 client contract and the F3
   server estimate depend on this answer.
2. **Sharing confirmation.** Is a chat instruction enough to trigger
   `share_memory`, or does the operator also have to click "Enviar"
   in the bandeja?
3. **Team approval.** Single approver by role, or consensus across
   the 5 operators? How many votes to approve? This blocks F3.
4. **Team rejection visibility.** If the team rejects a memory, does
   the author see it in their bandeja with the reason? Proposal: yes.
5. **Operator identity.** Comes from Windows (preferred for 5
   non-developers) or typed manually in `yhat-agent install`? Decide
   before the F1 installer writes `config.yaml`.
6. **`yhat-mcp-server` coexistence.** There is an existing Node MCP
   server that talks to `mssql`. Document what it does and how the
   new local Go server shares the registry slot.
7. **Sync cadence.** Pull on Claude Desktop open plus every 15
   minutes? Or per-call? Proposal: open + every 15 min.

These 7 are copied verbatim from the PRD. The F1 implementation
will guess the others, but cannot invent answers to 1, 2, 3, 5
without the user confirming.

## F1 scope (capture + inbox), estimated 5-6 days

- `internal/store` with SQLite WAL, migrations under
  `internal/store/migrations/001_init.sql`, and the schema from the PRD
  including `memories` + `upload_queue` + `sync_state`.
- Replace the synthetic FTS5 store in `internal/spike` with a real
  file-backed store at `%USERPROFILE%\.yhat\yhat.db`.
- New MCP tool `propose_memory` (decision / rule / anomaly / improvement,
  with title 1-200 chars, content 1-10000, optional context, plus
  sensitive-content validation and duplicate-by-`content_hash`).
- New MCP tool `list_pending` showing everything the operator has
  not yet approved.
- New MCP tool `get_memory` returning one memory with origin and
  history.
- `internal/bandeja` HTTP server on `127.0.0.1` with a random port,
  one-shot token in the URL, auto-shutdown after 15 min idle,
  embedded HTML for Aprobar / Rechazar (con motivo) / Editar, and an
  Enviados section.
- The `--bandeja` CLI flag opens the local browser via `xdg-open` /
  `open` / `start`.
- `config.yaml` and `state.json` under `%USERPROFILE%\.yhat\`,
  with the operator identity from open question 5.
- `BR1-BR13` business rules from the PRD enforced in the store and
  the bandeja, not in instructions to the agent.

## F2 scope (share + pull), estimated 4-5 days, after F1

- `share_memory` enqueues approved memories to `upload_queue`.
- `sync` subcommand runs in the background on Claude Desktop open
  and every 15 min (open question 7).
- Upload uses the API contract from the F3 service (open question 1).
- Cursor in `sync_state` so partial failures do not re-upload
  already-synced memories.
- Bandeja shows the state of each enqueued share.

## F3 scope (team approval), estimate pending answers 1 + 3

Server side, separate repository. The F1 client only needs the
contract that F2 documents.

## F4 (Power BI) and F5 (semantic search)

Deferred until the 5 operators are actually using the capture flow.
Gating on real data, not on technology choices.

## Backlog items not yet scheduled

- `yhat-agent update` for in-place binary replacement on Windows,
  with rollback if `--selftest` after the swap fails.
- Migration path for the previous Engram-based assets already
  present in `assets/agents/yhat-memory-capture.md` and the
  `assets/skills/yhat-memory-capture/SKILL.md`. The current
  `yhat-agent install` keeps writing them; the F1 store will
  consult the local SQLite first and only fall back to the
  embedded assets if the DB is empty.
- Documentation for the 5 non-developer operators: how to install
  the binary, how Claude Desktop picks it up, what the bandeja
  looks like.
- GitHub Actions release workflow that produces a Windows `.exe`
  plus a SHA-256 manifest and uploads it as a release asset.

## Status of native review

Lineage `review-a921ac9aaf9b4d74` ran all four lenses and surfaced
two candidate-caused findings (R3, R4). Corrections applied. Final
validation capture repeatedly rejected the reoffered binding as
stale; user disabled review mode at clone scope. No review
approval. A new session can re-enable and retry, or the user can
abandon the lineage and start a fresh one for F1.
