# import-engram-prospects

Objective: `yhat-agent import-engram` reads each person's local Engram and creates prospects (memories with status `proposed`) for the source of truth.
Branch: feat/import-engram (from dev). Delivery: ask-on-risk. Test runner: `go test ./...` (and `scripts/ci-local.sh`).

## Specs

- S1: "yo lo que quiero es que este agente saque los prospectos para la Fuente de la verdad" (each team member has their own local Engram).
- S2: The agent reads the person's Engram SQLite directly, read-only (`~/.engram/engram.db`, `ENGRAM_DATA_DIR` overrides the directory), and remembers how far it read so only new or changed observations are brought.
- S3: Prospect criterion: observations of the Engram project `yhat-knowledge` plus any observation whose `topic_key` starts with `yhat.`. Soft-deleted observations are ignored.
- S4: Widen the memory `type` from 4 to 12 values: decision, rule, anomaly, improvement (existing) plus observation, assumption, process, mapping, exception, entity-definition, integration, technical-rule. Existing databases migrate without losing rows.
- S5: The author of each prospect is the operator who runs the import (the configured operator).
- S6: Prospects are stored as `proposed` and are never validated or shared automatically; duplicates (same content hash) are skipped; the sensitive-content filter applies before storing.
- S7: `import-engram` has `--dry-run` and `--db <path>`; the help text and docs describe it.

## Tasks

- [x] T1 (S4) Migration 002 widening `memories.type`; Go validation, MCP enum, central-api doc. Route: inline. Risk: item 1 (migration of stored data), item 3 (type values consumed by yhat-central). Evidence: tests in internal/store/migration_types_test.go (RED then GREEN), `go test ./...` green.
- [x] T2 (S2, S3) `internal/engram` read-only reader with watermark query. Route: inline. Evidence: internal/engram tests green; migration failure/rollback and foreign_keys tests added (the independent verifier had no tools and returned UNVERIFIED, so those probes were run inline).
- [x] T3 (S3, S5, S6) Mapping Engram observation to memory; dedupe and sensitive filter. Route: inline. Evidence: cmd/yhat-agent/import_engram_test.go (RED: undefined, then GREEN).
- [x] T4 (S2, S6, S7) `import-engram` command, watermark in `sync_state`, help and docs. Route: inline. Evidence: dry run and real import against the local Engram into a scratch home: 16 prospects, second run 0, Engram file hash unchanged.
- [~] T5 README done; the Archify map is NOT updated: adding an Engram node collided with the existing routes under the showcase quality profile and needs a layout pass. Route: inline.

## Log

- L1 (2026-11-16): "es que todos en el equipo usamos engram ... entonces de este lugar queria sacar la informacion estructurada para Yhat" and "cada uno de mi equipo tiene engram. Entonces yo lo que quiero es que este agente saque los prospectos para la Fuente de la verdad".
- L2 (2026-11-16): Decisions from the user: read the SQLite directly; widen the types to the skill's set; author is the operator who imports; criterion is project `yhat-knowledge` plus `yhat.*` keys.
- L3 (2026-11-16): Evidence from the local Engram: 422 observations, only 1 with a `yhat.*` key, 15 in project `yhat-knowledge`; the rest are development notes.
- L4 (2026-11-16): Open risk: yhat-central (F3) validates `type` too (docs/central-api.md line 184). Sharing a memory with a new type may be rejected until the central service accepts it.
