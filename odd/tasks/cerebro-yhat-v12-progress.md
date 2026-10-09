# Cerebro YHat v1.2 — progress diagram (2026-10-09)

## Phases at a glance

```
F0 ──► F1 ──► F2 ──► F3 ──► F4 ──► F5
●      ◐      ○      ○      ○      ○
done   40%    open   open   later  later
```

● done   ◐ in progress   ○ not started   later = deferred

## Branch and tag map

```
main (origin)              2b9ebbc
   │
   ├── tag v0.2.0-f0 ──────── f4b9a61  (F0 verified, WorkSpace OK)
   │
   └── branch feat/cerebro-f1  60ebc94
         │
         ├── 32a4380 feat(store)  ── F1-A persistent store
         ├── 5fe8e30 docs(f1)     ── F1-A task doc
         ├── 52df538 feat(mcp)    ── F1-B real MCP tools
         ├── 2c17cf3 refactor     ── F1-B dead-code + subprocess fix
         └── 60ebc94 docs(f1)     ── F1-B task doc
```

## F1 sub-task map

```
F1 ─┬─ A  ● Store SQLite + migrations + BR1/3/5/7/9/11  (32a4380)
    │
    ├─ B  ● 4 MCP tools + queries + sentinel map            (52df538 + 2c17cf3)
    │      ├─ propose_memory      cmd/yhat-agent/mcp.go:219
    │      ├─ list_pending        cmd/yhat-agent/mcp.go:270
    │      ├─ get_memory          cmd/yhat-agent/mcp.go:317
    │      └─ search_brain        cmd/yhat-agent/mcp.go:350
    │
    ├─ C  ◐ Rewrite yhat-agent install
    │      └─ %USERPROFILE%\.yhat\ bin\ config.yaml state.json
    │      └─ status + bandeja subcommands
    │
    ├─ D  ○ Bandeja HTTP local
    │      └─ 127.0.0.1:0, one-shot token, 15 min idle
    │      └─ Aprobar / Rechazar(con motivo) / Editar / Enviados
    │
    ├─ E  ○ Sensitive-content filter (BR13)
    │      └─ replaces F1-B placeholder
    │
    └─ F  ○ WorkSpace regression run
           └─ F0 + F1 together on the 5 WorkSpaces
```

## File map (state after F1-B)

```
github.com/ArcKelMiranda/yhat-agent
├── cmd/yhat-agent
│   ├── main.go                (install / status / update / uninstall)
│   ├── mcp.go                 (F0 fts5_search + F1 4 tools)
│   ├── mcp_test.go            (46+ tests, F0 + F1)
│   ├── install_mcp.go         (F0 OpenCode + Claude registration)
│   └── install_mcp_test.go
├── internal
│   ├── clients
│   │   ├── clients.go         (F0 detection + safe write + duplicate-key)
│   │   └── clients_test.go
│   ├── spike
│   │   ├── search.go          (F0 synthetic FTS5 fixtures, F1-B fallback)
│   │   └── search_test.go
│   └── store
│       ├── store.go           (F1-A Open + Close + BR1/3/5/7/9/11)
│       ├── store_test.go      (14 BR-rule tests)
│       ├── queries.go         (F1-B FTS5 helpers)
│       └── migrations/001_init.sql
├── docs
│   ├── cerebro-f0.md
│   └── workspace-install.md
├── odd/tasks
│   ├── cerebro-local-f0.md
│   ├── workspace-mcp-registration.md
│   ├── cerebro-yhat-v12-roadmap.md
│   └── cerebro-f1.md          (F1 sub-task tracker)
└── (PRD-yhat-mcp-v1.1-revisado.md renamed for go install zip)
```

## BR-rule enforcement map (as of F1-B)

```
BR1  default status 'proposed'                   internal/store/store.go
BR3  validated_by/at non-null together           internal/store/store.go
BR5  title 1-200, content 1-10000                internal/store/store.go
BR7  unique content_hash for proposed+valid     internal/store/store.go
                                                 + idx_memories_live_hash
BR9  share_status only with validated/archived   internal/store/store.go
BR11 team records not editable via public API    internal/store/store.go
BR13 sensitive-content filter                    F1-E (placeholder active)
```

## Decision map (PRD answers, observation 411)

```
Q1  central repository        ──► new dedicated service (F3 greenfield)
Q2  sharing trigger            ──► chat only (no Enviar button)
Q3  team approval              ──► 3 of 5 operators majority
Q4  rejection visibility       ──► yes, with reason (F1-D + F2 sync)
Q5  operator identity          ──► Windows %USERNAME% (F1-C)
Q6  yhat-mcp-server coexist    ──► parallel, MCP name 'yhat'
Q7  sync cadence               ──► open + every 15 min (F2)
```

## What this means for the next session

```
F1-D (bandeja)         depends on F1-C (config.yaml + state.json) for the operator identity
F1-C (instalador)      depends on F1-E (filter) for the Aprobar/Rechazar copy
F1-F (regresión)       depends on F1-D for the approve/reject end-to-end
F2 (share + sync)      depends on F1-C (config with central URL placeholder) and F1-F
F3 (server)            depends on F2 contract
```

Suggested order to finish F1: F1-E -> F1-C -> F1-D -> F1-F.
