# Refresh the Archify architecture map for Cerebro YHat v1.2

## Specs

- S1. The user asked, verbatim: "mira, puedes decirme como llevamos este proyecto, si puedes utiliza archify para poderte entender mejor por favor"
- S2. The user asked, verbatim: "creamelo con esta skill https://github.com/tt-a1i/archify"
- S3. The architecture map must be produced with the pinned Archify skill at `tools/archify` (submodule `tt-a1i/archify`, commit `72c750bb070d95171dbb2244e5b62b1b7da69c12`), invoked as `node tools/archify/archify/bin/archify.mjs`.
- S4. The map must describe the current product, not the v0.1 installer CLI: MCP capture, sensitive filter, local SQLite store, human approval bandeja, share and sync, the central F3 service, the installer and update flow.
- S5. Every component must carry repository evidence (`sources`) that Archify verifies against the local checkout.
- S6. The delivered HTML must pass `deliver` with the showcase quality profile: 9 artifact checks, 0 errors, 0 warnings.
- S7. Regeneration must be deterministic: `node docs/architecture/generate.mjs` must reproduce the same artifact bytes.

## Tasks

- [x] T1 — Gather v1.2 architecture facts from the repository (read-only exploration, evidence with `path:line`). S3, S4, S5.
- [x] T2 — Author `docs/architecture/yhat-agent.architecture.json` for v1.2 (12 components, 11 connections, 4 cards, 2 guided views). S4, S5.
- [x] T3 — Validate with `archify validate --quality showcase --repo-root .` until 0 errors and 0 warnings. S6.
- [x] T4 — Deliver the HTML and collect the deterministic receipt. S6, S7.
- [x] T5 — Update `docs/architecture/generate.mjs` (showcase profile) and the README architecture section. S7.
- [x] T6 — Commit the work unit on `main`. Push stays a user decision.

## Log

- L1. Original request, verbatim: "mira, puedes decirme como llevamos este proyecto, si puedes utiliza archify para poderte entender mejor por favor".
- L2. "creamelo con esta skill https://github.com/tt-a1i/archify" — the user asked for the diagram to be created with the Archify skill.
- L3. Decision: replace the v0.1 map in place (same path, same README and generation wiring) instead of adding a second artifact, because the old map described a product that no longer exists.
- L4. Decision: `meta.repository` uses `link_mode: "local-only"` and no `provider`, because the local origin is `git@github-agent:ArcKelMiranda/yhat-agent.git` and Archify only accepts `github.com` or `gitee.com` as providers.
- L5. Layout: the first authored layout failed `composition/desktop-readability` because a 1530px viewBox projected node text to 5.47px at 1440x900 (minimum 6px). The layout was compacted to a 1230px content width, which passes.
- L6. Browser evidence: `archify visual-check` was attempted and reports `viewer/chrome-unavailable` (no Chrome or Chromium on this host, `ARCHIFY_CHROME` unset). Deterministic artifact checks passed; perceptual review is not claimed.

## Verification results

```
$ node tools/archify/archify/bin/archify.mjs validate architecture \
    docs/architecture/yhat-agent.architecture.json --quality showcase --repo-root . --json
ok: true

$ node tools/archify/archify/bin/archify.mjs deliver architecture \
    docs/architecture/yhat-agent.architecture.json \
    docs/architecture/yhat-agent.architecture.html --quality showcase --repo-root . --json
ok: true
specification: sha256 0c00003226f269d5a0cdf0695173838a148f3d0b602e58a14530b8a643281c1f (11439 bytes)
artifact:      sha256 814ccba5f6d06c36cb35535f5865d4dcf5c0dc53d06b646ffa3bff47cf730dfa (821610 bytes)
validation:    9/9 checks, showcase, 0 errors, 0 warnings
evidence:      verified, 26 references at revision 96dc34955e88e0b613f372eb74e462b77c728c42

$ node docs/architecture/generate.mjs
Archify HTML generated successfully. Size: 802.4 KB

$ sha256sum docs/architecture/yhat-agent.architecture.html   # before and after regeneration
814ccba5f6d06c36cb35535f5865d4dcf5c0dc53d06b646ffa3bff47cf730dfa

$ node tools/archify/archify/bin/archify.mjs visual-check \
    docs/architecture/yhat-agent.architecture.html --json
ok: false, status "skipped", code viewer/chrome-unavailable
```

## Map content (v1.2)

| Component | Type | Evidence |
|---|---|---|
| AI Clients (OpenCode, Claude Desktop) | frontend | `internal/clients/clients.go:71`, `:297` |
| MCP Server (stdio, 6 tools) | backend | `cmd/yhat-agent/mcp.go:612`, `:654`, `:737` |
| Sensitive Filter (BR13) | security | `internal/sensitive/sensitive.go:73`, `cmd/yhat-agent/mcp.go:230` |
| Local Store (SQLite WAL) | database | `internal/store/migrations/001_init.sql:6`, `:41`, `internal/store/store.go:172` |
| Share and Sync | backend | `internal/share/share.go:78`, `internal/share/transport.go:105`, `:245` |
| yhat-central (F3) | cloud | `docs/central-api.md:112`, `:285` |
| yhat-agent CLI | frontend | `cmd/yhat-agent/main.go:47` |
| Assets and Manifest | database | `embed.go:12`, `manifest.go:14` |
| Operator | external | `internal/bandeja/bandeja.go:197` |
| Bandeja (approval UI) | frontend | `internal/bandeja/bandeja.go:94`, `:81`, `:137` |
| GitHub Releases | cloud | `update.go:96`, `:184` |
| Update Candidate (`.yhat-agent-new`) | database | `update.go:292`, `paths.go:141` |
