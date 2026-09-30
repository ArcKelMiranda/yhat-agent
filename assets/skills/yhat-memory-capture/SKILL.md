---
name: yhat-memory-capture
description: "Capture YHat knowledge silently with enrichment queue, bootstrap mode, hygiene gate, compliance scoring, and audit mode. Triggered when the yhat-memory-capture agent is selected."
compatibility: opencode
version: "1.3.0"
---

# YHat Memory Capture Skill

Convert durable YHat knowledge emerged during conversation into Engram observations with silent capture, automatic enrichment from session context, hygiene gate validation, compliance scoring, and user-controlled question surfacing.

**Source of Truth**: YHat knowledge lives in the external `yhat-knowledge` system. Engram provides capture and staging infrastructure only — it does not promise automatic ingestion or synchronization. All Engram records are **staging records** proposed until a human confirms them. The agent never designates source of truth.

## Core Behavior

1. **Silent capture**: Save atomic facts immediately when user states them (after hygiene gate)
2. **Bootstrap mode**: Activate when Engram is empty or user requests foundational knowledge capture
3. **Hygiene gate**: Validate quality before save (pass/warn/fail)
4. **Compliance scoring**: Calculate completeness and quality score for each capture
5. **Coverage states**: Track whether knowledge is complete, partial, or incomplete
6. **Auto-enrichment**: Try to answer audit gaps from code, queries, files in session
7. **Ordered queue**: Questions ordered by compliance score (lowest first), age, business impact
8. **Adjacent knowledge**: Detect and link related knowledge
9. **User-controlled questions**: Surface questions only at pauses, explicit requests, or session close
10. **Rapid review**: Quick batch review workflow for multiple records
11. **Audit mode**: Generate read-only coverage and compliance reports
12. **Aging**: Track stale/aged knowledge

## Topic Key Pattern

All YHat observations use: `yhat.{type}.{domain}.{concept}`

### Topic Type Segments

| YHat Type | Segment | Description |
|-----------|---------|-------------|
| decision | `decision` | Architectural or design decisions |
| business-rule | `rule` | Business rules and regulations |
| observation | `obs` | General observations |
| assumption | `assum` | Unverified assumptions (needs review) |
| process | `process` | Workflows and processes |
| mapping | `map` | Entity relationships |
| exception | `excep` | Recurring bugs or exceptions |
| entity-definition | `def` | Entity definitions |
| integration | `intg` | System integrations |
| data-anomaly | `anom` | Data quality issues |
| technical-rule | `tech` | Technical standards |

### Domain Extraction

Domain is derived from the content context:
- `codes` — Code master data
- `fa` — Financial Agent / FA related
- `aum` — Assets Under Management
- `auth` — Authentication/authorization
- `branch` — Branch/branching logic
- `entidad` — Entity-related
- `sistemaa`, `sistemab`, `sistemac` — Fictitious system examples (EJEMPLO-NO-REAL)
- Or any meaningful domain from the content

### Dotted Key Tokenization

Dotted topic keys are tokenized for search. The `dotted_keys` metadata field contains individual segments:

```
topic_key: yhat.rule.codes.primary-key
dotted_keys: ["yhat", "rule", "codes", "primary-key"]
```

**Note**: The legacy `yhat.index.master` pattern is removed. Use explicit topic types.

### Topic Key Examples

| Content Topic | Topic Key |
|--------------|-----------|
| "FA display separator" | `yhat.rule.fa.display-separator` |
| "EntidadA to Branch relationship" | `yhat.map.entidada-branch` |
| "EntidadB to SISTEMAB mapping" | `yhat.map.entidadb-sistemab` |
| "Entity primary key rule" | `yhat.rule.entidad.primary-key` |
| "SISTEMAA system integration" | `yhat.intg.sistemaa` |
| "Duplicate detection logic" | `yhat.obs.duplicate-detection` |
| "Code creation process" | `yhat.process.codes.creation` |
| "Auth token expiry" | `yhat.excep.auth.token-expiry` |

## Engram Schema (Verified Fields)

### Native mem_save Fields

| Field | Value | Notes |
|-------|-------|-------|
| `project` | Active session project | From PI_SESSION_PROJECT or equivalent |
| `type` | `yhat-knowledge` | All YHat observations |
| `topic_key` | `yhat.{type}.{domain}.{concept}` | Pattern: 3-4 segments |
| `title` | Short, descriptive | Required, searchable |
| `content` | Knowledge + metadata | Full statement + YHat Metadata section |
| `scope` | `project` | Fixed scope |

### Metadata Inside content (NOT Native Fields)

All metadata goes inside `content` under `## YHat Metadata`:

```markdown
## YHat Metadata
- source: {business-user|ai-agent|validation-agent}
- original_type: {topic segment}
- confidence: {0.0-1.0}
- status: {proposed|confirmed|deprecated|contradicted|pending-enrichment}
- validated_by: {role/alias|pending}
- validated_at: {ISO timestamp}  # set by human only
- captured_at: {ISO timestamp}
- review_after: {ISO timestamp}
- evidence: {table.field, file, ticket reference}
- related_topic: {topic_key|null}
- supersedes: {topic_key|null}
- adjacent_topics: [{topic_key}, ...]
- compliance_score: {0-100%}
- coverage_state: {sin registros|registrado sin validar|validado}
- hygiene_gate: {pass|warn|fail}
- bootstrap_mode: {true|false}
- aged_at: {ISO timestamp|null}
- aging_reason: {string|null}
- dotted_keys: [{segment}, ...]
- is_example: {true|false}  # true for fictitious examples (never captured)
```

## Step 0: Pre-Capture Verification

Before any capture, execute these checks:

### 1. Engram Availability Check

```javascript
// Verify mem_save, mem_update, mem_search are available
// If unavailable → warn immediately, do NOT fake saves
```

### 2. Dotted Search Verification

```javascript
mem_search({
  project: activeProject,
  query: "yhat.rule.codes",
  type: "yhat-knowledge",
  limit: 5
})
// Verify partial matches work
```

### 3. Filter Verification

```javascript
mem_search({
  project: activeProject,
  query: "yhat",
  type: "yhat-knowledge",
  scope: "project",
  limit: 10
})
// Verify type, scope, and project filters work
```

### 4. mem_update Works

```javascript
// Verify mem_update can modify existing records
// Test with a known topic_key if available
```

### 5. Project Binding

```javascript
// Verify that saves to unrelated projects are rejected
// mem_save with project != activeProject should fail or be rejected
```

### 6. Bootstrap Mode Detection

**Bootstrap Mode** activates when:
- Engram returns no existing yhat-knowledge records
- User explicitly requests bootstrap ("bootstrap", "inicializar", "fundamentos")
- Session starts with bootstrap keyword

In bootstrap mode:
- Initial summary skipped (nothing to summarize)
- Captured facts include `bootstrap_mode: true`
- User prompted for foundational knowledge areas
- Hygiene gate relaxed (warn instead of fail)

### 7. Duplicate/Contradiction Check

Before capturing new knowledge:

```javascript
// Search yhat-knowledge read-only for duplicates
mem_search({
  project: activeProject,
  query: specificConcept,
  type: "yhat-knowledge",
  limit: 10
})

// Check for contradictions:
// - Same topic_key, different content → contradiction
// - Same domain/concept, different rules → potential conflict
```

## Hygiene Gate

Before any save, evaluate hygiene criteria:

### Gate Criteria

| Criterion | Pass | Warn | Fail |
|-----------|------|------|------|
| Topic key valid | Matches `yhat.{type}.{domain}.{concept}` | Partial match | Invalid or missing |
| Evidence present | Has evidence field | Evidence is weak (<0.7 confidence) | No evidence |
| Content quality | 50+ chars, clear statement | 20-50 chars | <20 chars |
| No secrets | Clean scan | Suspicious patterns | Contains secrets |
| Type appropriate | Type matches content | Close match | Type mismatch |
| Dotted keys | Extracted and included | Partial extraction | Not extracted |
| Tags line | Literal `tags:` line present in content | — | No `tags:` line found |
| No local paths/users | No absolute paths like `/home/user` or real names | — | Contains absolute local paths or user names |

### Gate Result Handling

```javascript
// hygiene_gate metadata
hygiene_gate: "pass" | "warn" | "fail"

// If fail → do not save, report issue to user
// If warn → save with warning, surface in audit
// If pass → save normally
```

## Compliance Score

The compliance score is a 0–100% value measuring hygiene and completeness. It is calculated from exactly **five binary checks**:

| Check | Pass Condition |
|-------|---------------|
| Valid topic_key | Matches `yhat.{type}.{domain}.{concept}` |
| Evidence | `evidence` field is present and non-null |
| Status | `status` is set (not empty) |
| Tags line | Literal `tags:` line present in content body |
| Complete metadata | `validated_by`, `captured_at`, and either `evidence` or `related_topic` are present |

**Formula**: `compliance_score = 100 × passed_checks / 5`

Result: 0%, 20%, 40%, 60%, 80%, or 100%

**Confidence** is a separate metadata field (0.0–1.0 scale) and does not affect the compliance score.

### Coverage States

| State | Description |
|-------|-------------|
| `sin registros` | No records exist for this topic |
| `registrado sin validar` | Records exist but not yet confirmed by human (`status: proposed`) |
| `validado` | Records confirmed by human (`status: confirmed`, `validated_by`, `validated_at`) |

## Capture Protocol

### Step 1: Initial Check (On Activation — unless bootstrap mode)

```
1. Check if Engram tools are available
2. mem_search({ project: activeProject, query: "yhat", type: "yhat-knowledge", limit: 50 })
3. Summarize: existing domains, compliance summary, empty areas, pending questions count
4. NO questions
```

### Step 2: Identify Knowledge

Stay attentive for:
- Business rules stated explicitly
- Decisions made about data handling
- Entity relationships and mappings
- Process definitions
- Exceptions or anomaly patterns
- User corrections of data

### Step 3: Validate Eligibility

### MUST Capture

✅ Business rules and decisions
✅ Verified facts from operational systems
✅ Entity relationships and mappings
✅ Process descriptions
✅ Exception handling rules
✅ Data quality observations

### MUST Reject

❌ **Secrets**: passwords, API keys, tokens, bearer, ENV vars
❌ **Transient**: session summaries, transcripts, debug logs
❌ **Personal Data**: user paths, local configs, session state
❌ **Chatty Content**: greetings, acknowledgements, off-topic

### Step 4: Vocabulary → Topic Mapping

| Keywords (ES/EN) | Topic Segment |
|-----------------|---------------|
| decisión, arquitectura, decision, architecture | `decision` |
| regla de negocio, business rule | `rule` |
| bugfix, bug, error | `excep` |
| definición, definition, entity | `def` |
| relación, mapping, relationship | `map` |
| proceso, workflow, process | `process` |
| creo que, asumo, I think, I assume | `assum` |
| encontré, found, noticed, observed | `obs` |
| datos mal, data wrong, data issue | `anom` |
| integración, integration, connects to | `intg` |
| default pattern, technical standard | `tech` |
| (unknown) | `obs` |

### Step 5: Hygiene Gate Evaluation

```
1. Check topic key validity
2. Check evidence presence
3. Check content quality
4. Scan for secrets
5. Verify type appropriateness
6. Extract dotted keys
7. Calculate compliance score
8. Determine coverage state
```

### Step 6: Duplicate Check

```
1. Search yhat-knowledge for similar topic_keys
2. Check for content contradictions
3. If duplicate → update existing record
4. If contradiction → mark old as deprecated, save new
```

### Step 7: Silent Save

```javascript
// Immediately after user states a fact
mem_save({
  project: activeProject,
  type: "yhat-knowledge",
  topic_key: "yhat.map.entidada-branch",
  title: "EntidadA to Branch mapping",
  content: `EntidadA entities map to Branch 'SUC-A001' for the SISTEMAA system.

tags: mapping, entidada, sistemaa, branch
status: proposed

## YHat Metadata
- source: business-user
- original_type: mapping
- confidence: 0.95
- status: proposed
- validated_by: pending
- captured_at: ${new Date().toISOString()}
- review_after: ${futureDate(7)}
- evidence: "EntityMapping table, BranchCode column"
- related_topic: null
- adjacent_topics: ["yhat.map.entidadb-branch"]
- compliance_score: 60
- coverage_state: registrado sin validar
- hygiene_gate: pass
- bootstrap_mode: false
- dotted_keys: ["yhat", "map", "entidada", "branch"]`,
  scope: "project"
})

// One-line confirmation only
"Guardado: yhat.map.entidada-branch"
```

### Step 8: Evaluate Enrichment Gaps

For each saved fact, evaluate:
- Business rationale (why this rule exists)
- Scope and exceptions
- Origin (which system/table/field)
- Validity period
- Owner who can validate
- Real example
- Relation to other records (adjacent topics)

### Step 9: Auto-Enrichment

Before enqueuing a question, try to answer from session context:

```javascript
// If user shared code with the mapping
if (userSharedCode.includes("BranchCode")) {
  // Update the existing record
  mem_update({
    id: existingRecordId,
    content: existingContent + "\n- evidence: \"Code: EntityService.getBranchCode()\""
  })
  // Do NOT mark as confirmed
}
```

### Step 10: Adjacent Knowledge Detection

```javascript
// Detect adjacent topics:
// 1. Same domain, different concept
// 2. Same concept, different domain
// 3.上下游 relationships

// Add to metadata:
adjacent_topics: ["yhat.map.entidadb-branch", "yhat.rule.entity.validation"]
```

### Step 11: Queue Management

If cannot auto-enrich, save question as:

```javascript
mem_save({
  project: activeProject,
  type: "yhat-knowledge",
  topic_key: "yhat.assum.entidada.sistemaa-scope",
  title: "SISTEMAA scope for EntidadA mapping",
  content: `Question: Does the EntidadA to Branch mapping apply to SISTEMAB as well?

Related to: yhat.map.entidada-branch

tags: assumption, mapping, entidada, sistemab
status: pending-enrichment

## YHat Metadata
- source: ai-agent
- original_type: assumption
- confidence: 0.4
- status: pending-enrichment
- validated_by: pending
- captured_at: ${new Date().toISOString()}
- review_after: ${futureDate(3)}
- evidence: null
- related_topic: "yhat.map.entidada-branch"
- adjacent_topics: ["yhat.map.entidadb-branch"]
- compliance_score: 40
- coverage_state: registrado sin validar
- hygiene_gate: warn
- bootstrap_mode: false
- dotted_keys: ["yhat", "assum", "entidada", "sistemaa-scope"]
- is_example: true`,
  scope: "project"
})
```

### Step 12: Queue Ordering

Order questions by:
1. Compliance score (lowest first)
2. Age (oldest first)
3. Business impact

## When to Surface Questions

### Natural Pause
User says: "listo", "gracias", "eso es todo", "terminamos", or changes topic clearly.

```
Tengo ${queue.length} preguntas para completar lo que capturé (ordenadas por compliance).
¿Las vemos ahora o las dejo para después?
```

### Explicit Request
User says: "enriquecé", "qué te falta", "hacéme preguntas", "revisá rápido", "rapid review"

```
${formatOrderedQueue(queue)}
```

Or activate Rapid Review for batch processing.

### Real Blockage
Ambiguity would make the record incorrect AND cannot continue.

```
Solo una pregunta sobre esto: [brief question]
```

### Session Close
See Session Close Protocol below.

## Question List Requirements

Every question list must:
- Contain the first question exactly: `¿Existe documentación oficial viva (SharePoint, wiki, esquema versionado) que valide este dominio?`
- Have at most five questions, one line each
- Include the record identifier (topic_key)
- Keep 14-day aging requirement for stale questions
- Top N examples must have exactly N items (no truncation, no padding)

## Ordered Queue Format

```
Tengo ${n} preguntas para cerrar lo que capturé (ordenadas por compliance score):

1. [yhat.assum.api-response] (compliance: 40%)
   ¿Existe documentación oficial viva (SharePoint, wiki, esquema versionado) que valide este dominio?
   Razón: Baja compliance — necesita evidencia

2. [yhat.map.entidada-branch] (compliance: 60%)
   ¿Este mapeo aplica también para el sistema SISTEMAB?
   Razón: Diferentes sistemas pueden tener reglas distintas

3. [yhat.rule.codes.primary-key] (compliance: 80%)
   ¿Cuál es el rango de fechas de vigencia de este mapeo?
   Razón: Afecta cálculos de posiciones

Respondé "skip" para las que no puedas responder ahora.
```

## Rapid Review Workflow

Activated with: "rapid review", "revisá rápido", "quick review"

```
## Rapid Review — Batch 1/5
### Project: {activeProject}

### yhat.rule.codes.primary-key
- Compliance: 80% | Coverage: registrado sin validar | Hygiene: pass
- Confidence: 0.95 | Status: proposed

"Entities in SISTEMAA use composite key (EntityId, SourceSystem, EffectiveDate)."

Evidence: Entity.java, @EmbeddedId annotation

Adjacent: yhat.rule.entidad.effective-dates

[A]pprove | [D]eprecate | [S]kip | [E]dit
```

## Session Close Protocol

```
Al cerrar la sesión:

1. Listar hechos durables sin guardar → ofrecer guardarlos
2. Ejecutar aging check en registros con review_after + 30 días excedido
3. Mostrar cola: "Tengo ${n} preguntas pendientes (compliance promedio: ${avg}). ¿Las respondemos ahora?"
4. Si el usuario dice "después" → no insistir en esta sesión
5. La próxima sesión: mencionar cola pendiente en resumen inicial
```

## Audit Mode

Activated with: "auditá yhat", "yhat audit", "reporte de yhat", "audit"

### Report Format

```
## YHat Audit Report — ${date}

### Projects Inspected
- ${activeProject} (current session)

### Coverage
- Total records: ${total}
- By type: decision(${n}), rule(${n}), map(${n}), obs(${n}), assum(${n})
- By domain: entidada(${n}), entidadb(${n}), codes(${n}), entidad(${n})

### Compliance Summary
- Average compliance score: ${avg}%
- Records with compliance < 50%: ${n}
- Records with hygiene_gate = warn: ${n}
- Records with hygiene_gate = fail: ${n} (blocked)

### Coverage States
- Sin registros: ${sin_registros}
- Registrado sin validar: ${registrado_sin_validar}
- Validado: ${validado}

### Pending Review
- status: proposed → ${n}
- status: pending-enrichment → ${n}
- review_after < today → ${n}
- Aged records (review_after + 30 days exceeded) → ${n}

### Quality
- confidence < 0.7 → ${n}
- Sin evidence → ${n}
- hygiene_gate = warn → ${n}

### Enrichment Queue
- Preguntas abiertas → ${n}
- Sin respuesta por 14+ días → ${n}
- Compliance promedio de la cola → ${avg}

### Human Confirmation
- Registros con status: confirmed (confirmados por humano) → ${n}
- Registros con status: proposed (pendientes de confirmación) → ${n}

### Adjacent Knowledge
- Registros con adjacent_topics → ${n}
- Registros huérfanos (sin enlaces) → ${n}

### Coverage Gaps
- Dominios sin conocimiento: ${list}

### Top 10 Preguntas por Compliance
1. [pregunta] → [topic_key] (compliance: ${score})
...
```

## Aging

Knowledge can be marked as aged:

```javascript
// Triggers:
// 1. User marks as aged
// 2. review_after exceeded by 30+ days
// 3. Contradicted by new knowledge

aged_at: "2025-01-20T10:00:00Z"
aging_reason: "SISTEMAA v2.0 changed entity key behavior"
```

### Aging Check

```
// Run at session start and close
// Mark records as aged when:
// - review_after + 30 days < today
// - Not already aged

aged_at: current_timestamp
aging_reason: "Exceded review window by 30+ days"
```

## Fictitious Non-Business Examples (EJEMPLO-NO-REAL)

All examples use fictitious entities marked with `EJEMPLO-NO-REAL`. These records are **never captured**, listed as gaps, or asked about in the enrichment queue. Audit wording explicitly excludes them.

```
# Fictitious Examples — EJEMPLO-NO-REAL:
- Entities: EntidadA, EntidadB, EntidadC
- Systems: SISTEMAA, SISTEMAB, SISTEMAC
- Codes: COD-001, COD-002, COD-003
- Branches: SUC-A001, SUC-B002
- Users: usuario-a@ejemplo.com, usuario-b@ejemplo.com

# DO NOT USE:
- Real company names, real system names, real codes from production
- Real user identifiers or absolute local paths (/home/user, C:\\Users\\...)
- Old FA separator patterns or real CodeId+SourceSystem combinations
```

### Example Payloads (EJEMPLO-NO-REAL)

#### Valid: Business Rule

```json
{
  "project": "${activeProject}",
  "type": "yhat-knowledge",
  "topic_key": "yhat.rule.entidad.primary-key",
  "title": "Entity primary key composition",
  "content": "Entities in SISTEMAA are identified by a composite key of (EntityId, SourceSystem, EffectiveDate).\n\ntags: rule, entidad, primary-key, sistemaa\nstatus: proposed\n\n## YHat Metadata\n- source: business-user\n- original_type: business-rule\n- confidence: 0.95\n- status: proposed\n- validated_by: pending\n- captured_at: 2025-01-15T11:00:00Z\n- review_after: 2025-01-22T11:00:00Z\n- evidence: \"Entity.java, @EmbeddedId annotation\"\n- related_topic: null\n- adjacent_topics: [\"yhat.rule.entidad.effective-dates\"]\n- compliance_score: 80\n- coverage_state: validado\n- hygiene_gate: pass\n- bootstrap_mode: false\n- dotted_keys: [\"yhat\", \"rule\", \"entidad\", \"primary-key\"]",
  "scope": "project"
}
```

#### Valid: Mapping

```json
{
  "project": "${activeProject}",
  "type": "yhat-knowledge",
  "topic_key": "yhat.map.entidada-branch",
  "title": "EntidadA to Branch mapping",
  "content": "EntidadA entities map to Branch 'SUC-A001' for the SISTEMAA system.\n\ntags: mapping, entidada, sistemaa, branch\nstatus: proposed\n\n## YHat Metadata\n- source: business-user\n- original_type: mapping\n- confidence: 0.9\n- status: proposed\n- validated_by: pending\n- captured_at: 2025-01-15T10:00:00Z\n- review_after: 2025-01-22T10:00:00Z\n- evidence: \"EntityMapping table, BranchCode column\"\n- related_topic: null\n- adjacent_topics: [\"yhat.map.entidadb-branch\"]\n- compliance_score: 60\n- coverage_state: registrado sin validar\n- hygiene_gate: pass\n- bootstrap_mode: false\n- dotted_keys: [\"yhat\", \"map\", \"entidada\", \"branch\"]",
  "scope": "project"
}
```

#### Valid: Assumption with Enrichment Question

```json
{
  "project": "${activeProject}",
  "type": "yhat-knowledge",
  "topic_key": "yhat.assum.entidada.sistemaa-scope",
  "title": "SISTEMAA scope for EntidadA mapping",
  "content": "Question: Does the EntidadA to Branch mapping apply to SISTEMAB as well?\n\nRelated to: yhat.map.entidada-branch\n\ntags: assumption, mapping, entidada, sistemab\nstatus: pending-enrichment\n\n## YHat Metadata\n- source: ai-agent\n- original_type: assumption\n- confidence: 0.4\n- status: pending-enrichment\n- validated_by: pending\n- captured_at: 2025-01-15T10:05:00Z\n- review_after: 2025-01-18T10:05:00Z\n- evidence: null\n- related_topic: \"yhat.map.entidada-branch\"\n- adjacent_topics: [\"yhat.map.entidadb-branch\"]\n- compliance_score: 40\n- coverage_state: registrado sin validar\n- hygiene_gate: warn\n- bootstrap_mode: false\n- dotted_keys: [\"yhat\", \"assum\", \"entidada\", \"sistemaa-scope\"]",
  "scope": "project"
}
```

#### Rejected: Secret

```json
❌ { "content": "API key is stored as env var API_KEY=sk-123..." }
```

#### Rejected: Session Summary

```json
❌ { "content": "Session summary: discussed EntidadA mapping, reviewed SISTEMAA entity..." }
```

#### Rejected: Real Business Data

```json
❌ { "content": "EntidadA maps to SUC-A001 in production database..." }
```

## Validation Checklist

Before calling mem_save:

- [ ] `project` is active session project (not hard-coded)
- [ ] `type` is `yhat-knowledge`
- [ ] `topic_key` matches `yhat\.[a-z]+\.[a-z]+.*`
- [ ] `title` is short and descriptive
- [ ] `content` includes knowledge + YHat Metadata section
- [ ] `scope` is `project`
- [ ] No secrets/credentials in any field
- [ ] No session-specific paths
- [ ] One atomic fact per record
- [ ] Hygiene gate evaluated (pass/warn/fail)
- [ ] Compliance score calculated
- [ ] Coverage state determined
- [ ] Dotted keys extracted and included
- [ ] Human confirmation sets `status: confirmed`, `validated_by`, and `validated_at`
- [ ] No `source_of_truth` field (Engram records are staging only)
- [ ] Fictitious examples marked `EJEMPLO-NO-REAL` and `is_example: true`
- [ ] All examples use literal `tags:` line (not HTML comment)

## Version

1.3.0 — Coverage states corrected (sin registros/registrado sin validar/validado), fictitious examples marked EJEMPLO-NO-REAL, literal tags: line required, first question exact wording, source_of_truth removed, human confirmation required
