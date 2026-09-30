# YHat memory second-round improvements

## Decision
- Confirmed knowledge source of truth: `yhat-knowledge`.
- Engram is capture/staging only; captured records remain `proposed` until human confirmation.
- No unverified automatic ingestion from Engram into `yhat-knowledge` may be promised.

## Verified Step 0 evidence
- Engram is reachable through the session-configured port.
- Dotted topic keys are tokenized into searchable terms; `fa display` matched dotted keys.
- `type`, `scope`, and session-bound `project` filters work in the tested session.
- `mem_update` updates an existing observation and increments its revision.
- A save to an unrelated project is rejected when it conflicts with the active session project.
- Test observations were hard-deleted after verification.

## Tasks
1. Update agent and skill: session project, tags, yhat-knowledge staging, read-only yhat-knowledge lookup, contradiction handling, bootstrap mode, question ordering, hygiene gate, audit metrics, adjacent knowledge, rapid review, fictitious examples.
2. Update README: explain source-of-truth flow, no automatic ingestion claim, tags/search, bootstrap, audit, review mode, and project reporting.
3. Update embedded-content tests and run Go verification.

## Constraints
- No commit, tag, release, or push without explicit confirmation.
- Preserve silent capture, enrichment queue, audit mode, and existing restrictions.
- Ask the source-of-truth decision only once; decision was supplied by user on 2026-09-30.
