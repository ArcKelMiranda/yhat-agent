# yhat-agent

Installable Go CLI for the YHat knowledge capture agent and skill.

## Installation

```bash
# Install or update via go install (uses the latest released version)
go install github.com/ArcKelMiranda/yhat-agent/cmd/yhat-agent@latest

# Or download a pre-built binary from the Releases page
# https://github.com/ArcKelMiranda/yhat-agent/releases
```

## Usage

```bash
yhat-agent install   # Install embedded agent and skill assets
yhat-agent status   # Report installation state without making changes
yhat-agent update   # Download and verify latest release from GitHub
yhat-agent version  # Show version and build information
yhat-agent uninstall --yes   # Remove managed files (requires confirmation)
```

### Update behavior

`yhat-agent update` downloads binaries directly from GitHub Releases using the stable
`releases/latest/download/` redirect endpoint. No GitHub API token or rate-limited
endpoint is used. Each download is verified against its SHA-256 checksum before a
candidate binary is written alongside the running executable. The running executable
is never overwritten; users apply the update by replacing it manually after review.

## What it does

`yhat-agent` is a self-contained binary that installs the YHat OpenCode agent and skill
assets into your local OpenCode configuration directory. It tracks only its own files
and can safely update from GitHub Releases.

YHat captures durable operational knowledge (business rules, decisions, patterns, mappings)
during AI coding sessions with key behaviors:

### 1. Silent Capture with Hygiene Gate

The agent saves atomic facts immediately when you state them — no waiting until session end,
no interruptions. Before every save, a hygiene gate validates:
- Topic key validity (must match `yhat.{type}.{domain}.{concept}`)
- Evidence presence
- Content quality (minimum 50 characters)
- No secrets or credentials
- Type appropriateness
- Dotted key extraction

Gate results: `pass` (save normally), `warn` (save with warning), `fail` (block save).

**Hygiene requirement: literal `tags:` line.** Every fact record **must** include a literal YAML
line beginning with `tags:` followed by at least one tag value. This is not a suggestion — the
hygiene gate checks for a line that literally starts with `tags:` in the YAML frontmatter.
Scattered references to "tags" in prose comments do not satisfy this requirement.

### 2. Compliance Scoring

Every capture receives a compliance score (0–100%) calculated from **five binary checks**:

| Check | Pass Condition |
|-------|---------------|
| Valid topic_key | Matches `yhat.{type}.{domain}.{concept}` |
| Evidence | `evidence` field is present and non-null |
| Status | `status` is set (not empty) |
| Tags line | Literal `tags:` line present in content body |
| Complete metadata | `validated_by`, `captured_at`, and either `evidence` or `related_topic` are present |

**Formula**: `compliance_score = 100 × passed_checks / 5`

Result: 0%, 20%, 40%, 60%, 80%, or 100%

**Confidence** (0.0–1.0) is a separate metadata field and does not affect the compliance score.

Coverage states track registration status: `sin registros` (no records), `registrado sin validar` (proposed), `validado` (confirmed by human).

### 3. Bootstrap Mode

When Engram is empty or user requests "bootstrap", the agent:
- Skips initial summary (nothing to summarize)
- Includes `bootstrap_mode: true` on captures
- Prompts for foundational knowledge areas
- Relaxes hygiene gate (warn instead of fail)

### 4. Enrichment Queue

For each saved fact, the agent evaluates what is missing for full auditability (business
rationale, scope, exceptions, origin, validity, owner). Before asking you, it tries to
answer from code, queries, or files you shared in the session.

Questions are **ordered by compliance score** (lowest first), age, and business impact.
The agent only asks questions at moments you control:
- Natural pauses ("listo", "gracias", topic change)
- Explicit requests ("enriquecé", "qué te falta", "revisá rápido")
- Real blockage (one brief question only)
- Session close

### 5. Adjacent Knowledge

The agent detects and links related knowledge:
- Same domain, different concept
- Same concept, different domain
-上下游 relationships (upstream/downstream)

Adjacent topics are stored in `adjacent_topics` metadata for cross-referencing.

### 6. Rapid Review

For quick batch review of multiple records, activate with "rapid review":
```
## Rapid Review — Batch 1/5

### yhat.rule.entidad.primary-key
- Compliance: 85% | Coverage: registrado sin validar | Hygiene: pass
- Confidence: 0.95 | Status: proposed

"Entities in SISTEMAA use composite key..."

[A]pprove | [D]eprecate | [S]kip | [E]dit
```

### 7. Human Confirmation

Knowledge is captured and staged in Engram as `status: proposed`. Human confirmation
sets `status: confirmed`, `validated_by` (role/alias), and `validated_at` (date).
The agent never sets these fields. Engram records are staging only — the external
`yhat-knowledge` system is the authoritative source.

### 8. Aging

Records can be marked as aged/stale:
- User marks as aged
- `review_after` exceeded by 30+ days
- Contradicted by new knowledge

Aging check runs at session start and close.

### 9. Audit Mode

Activate with "auditá yhat" for a read-only coverage report:
- Records by type and domain
- Compliance summary (average score, low compliance count)
- Coverage states (sin registros/registrado sin validar/validado)
- Pending review and aged records
- Enrichment queue with compliance ordering
- Human confirmation status (confirmed vs. proposed)
- Adjacent knowledge links
- Coverage gaps

**Audit reports include "Projects Inspected"** showing which projects were queried.

## Source of Truth Model

**YHat knowledge lives in the external `yhat-knowledge` system**. Engram provides capture
and staging infrastructure only — it does not promise automatic ingestion or synchronization.
All Engram records are **staging records** proposed until a human confirms them.
The agent never designates source of truth.

Before every capture, the agent:
1. Searches `yhat-knowledge` read-only for duplicates/contradictions
2. Validates against existing knowledge
3. Marks duplicates for update, contradictions for deprecation
4. Human confirmation sets `status: confirmed`, `validated_by`, and `validated_at`

## Step 0: Pre-Capture Verification

The agent verifies these capabilities at startup:

1. **Engram availability**: `mem_save`, `mem_update`, `mem_search` functional
2. **Dotted search**: Partial key matching works
3. **Filter verification**: Type, scope, project filters functional
4. **mem_update works**: Can modify existing records
5. **Project binding**: Saves to unrelated projects rejected by session binding
6. **Bootstrap detection**: Empty Engram or explicit bootstrap request
7. **Duplicate check**: Read-only search before every capture

## Topic Pattern

All YHat observations use: `yhat.{type}.{domain}.{concept}`

| Type | Segment | Example |
|------|---------|---------|
| Decision | `decision` | `yhat.decision.fa-display` |
| Business Rule | `rule` | `yhat.rule.entity.primary-key` |
| Observation | `obs` | `yhat.obs.duplicate-detection` |
| Assumption | `assum` | `yhat.assum.entidada-sucursal.ejemplo-no-real-scope` |
| Process | `process` | `yhat.process.codes.creation` |
| Mapping | `map` | `yhat.map.codes.entidada-sucursal` |
| Exception | `excep` | `yhat.excep.auth.token-expiry` |
| Definition | `def` | `yhat.def.code-entity` |
| Integration | `intg` | `yhat.intg.sistemaa-ejemplo-no-real` |
| Data Anomaly | `anom` | `yhat.anom.missing-values` |
| Technical Rule | `tech` | `yhat.tech.retry-backoff` |

### Dotted Key Tokenization

Dotted topic keys are tokenized in `dotted_keys` metadata for partial matching searches:

```
topic_key: yhat.rule.entity.primary-key
dotted_keys: ["yhat", "rule", "entity", "primary-key"]
```

**Note**: The legacy `yhat.index.master` pattern is removed. Use explicit topic types.

### Fictitious Examples (EJEMPLO-NO-REAL)

All documentation and examples use fictitious entities marked with `EJEMPLO-NO-REAL`.
These records are **never captured**, listed as gaps, or asked about in the enrichment queue.

```
Entities: EntidadA, EntidadB, EntidadC
Systems: SISTEMAA, SISTEMAB, SISTEMAC
Codes: COD-001, COD-002, COD-003
Branches: SUC-A001, SUC-B002
```

**DO NOT USE**: Real company names, real system names, real codes from production,
real user identifiers, or absolute local paths.

## Filtering YHat Knowledge

```javascript
// All YHat observations (uses active session project)
mem_search({ project: activeProject, query: "yhat", type: "yhat-knowledge" })

// Dotted key search (tokenized)
mem_search({ project: activeProject, query: "yhat.rule.codes", type: "yhat-knowledge" })

// Pending enrichment questions
mem_search({ project: activeProject, query: "pending-enrichment", type: "yhat-knowledge" })

// Low compliance records
mem_search({ project: activeProject, query: "compliance", type: "yhat-knowledge" })

// Records needing review
mem_review({ action: "list", project: activeProject })
```

## Active Session Project

The agent uses the **active session project** from `PI_SESSION_PROJECT` environment
variable or equivalent. It does not hard-code `yhat` as the project name. This enables:
- Multi-project knowledge isolation
- Session-specific capture contexts
- Project-scoped search and review

## Benefits

- **Silent capture**: Saves facts immediately without interrupting your work
- **Hygiene gate**: Quality validation before every save (pass/warn/fail)
- **Compliance scoring**: Tracks completeness and quality of captured knowledge
- **Bootstrap mode**: Handles empty Engram or foundational knowledge capture
- **Ordered questions**: Prioritized by compliance score, age, and business impact
- **Adjacent knowledge**: Detects and links related topics
- **Rapid review**: Quick batch processing of multiple records
- **Aging**: Tracks stale and outdated knowledge
- **Human confirmation**: Records start as proposed; human sets confirmed status, validated_by, validated_at
- **User-controlled questions**: Surfaces enrichment queue only at pauses you control
- **Audit mode**: Generate coverage and quality reports on demand
- **EJEMPLO-NO-REAL examples**: Fictitious examples excluded from audit metrics
- **Active session project**: Session-scoped capture, not hard-coded

## Source layout

```
yhat-agent/
├── cmd/yhat-agent/          # CLI entry point
├── assets/                  # Embedded agent and skill assets
│   ├── agents/              # OpenCode agent definitions
│   │   └── yhat-memory-capture.md
│   └── skills/              # OpenCode skill definitions
│       └── yhat-memory-capture/
│           └── SKILL.md
├── *.go                     # Core logic: install, status, update, uninstall
├── *_test.go                # Test suite
└── go.sum                   # Go module checksums
```

## Architecture Documentation

The system architecture is documented as a self-contained rendered diagram and a
machine-readable source.

| Artifact | Description |
|---|---|
| [`docs/architecture/yhat-agent.architecture.json`](docs/architecture/yhat-agent.architecture.json) | Archify architecture source (evidence-backed) |
| [`docs/architecture/yhat-agent.architecture.html`](docs/architecture/yhat-agent.architecture.html) | Rendered standalone HTML |
| [`docs/architecture/generate.mjs`](docs/architecture/generate.mjs) | Deterministic generation script |
| [`docs/architecture/NOTICE.md`](docs/architecture/NOTICE.md) | Upstream licenses and provenance |

**Regenerate the HTML:**

```bash
node docs/architecture/generate.mjs
```

**Initialize the Archify submodule** (required before first generation):

```bash
git submodule update --init tools/archify
```

## License

**Proprietary Commercial Software — All Rights Reserved**

The Software and its source code in this repository are proprietary and confidential
trade secret property of **ArcKelMiranda**. This is not open-source or free software.
Source code may be publicly visible in this repository; such visibility does not waive
or forfeit ArcKelMiranda's trade secret rights or proprietary interests in the Software.

**No rights are granted by repository access.**

Any right to install, use, modify, integrate, or otherwise exploit this software
— in whole or in part — requires a separately executed commercial agreement or
order form with ArcKelMiranda, and any applicable fees must be paid and kept current.

Unauthorized use, copying, redistribution, or sublicensing of the source code is
strictly prohibited and may constitute a violation of applicable law.

To obtain authorized access or a commercial agreement, contact ArcKelMiranda through
the repository owner's official GitHub communication channels.

For the full license terms, see the [LICENSE](LICENSE) file.
