---
name: yhat-memory-capture
description: "Capture YHat knowledge silently with enrichment queue and audit mode. Triggered when the yhat-memory-capture agent is selected."
compatibility: opencode
version: "1.1.0"
---

# YHat Memory Capture Skill

Convert durable YHat knowledge emerged during conversation into Engram observations with silent capture, automatic enrichment from session context, and user-controlled question surfacing.

## Core Behavior

1. **Silent capture**: Save atomic facts immediately when user states them
2. **Auto-enrichment**: Try to answer audit gaps from code, queries, files in session
3. **Queue management**: Unanswered questions go to enrichment queue
4. **User-controlled questions**: Surface questions only at pauses, explicit requests, or session close
5. **Audit mode**: Generate read-only coverage reports

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
- `fa` — Financial Agent / FA related
- `codes` — Code master data
- `aum` — Assets Under Management
- `auth` — Authentication/authorization
- `branch` — Branch/branching logic
- `payment` — Payment processing
- `office` — Office/branch entities
- `ibd` — Investment Banking Division
- Or any meaningful domain from the content

### Topic Key Examples

| Content Topic | Topic Key |
|--------------|-----------|
| "FA display separator" | `yhat.rule.fa.display-separator` |
| "Code to FA relationship" | `yhat.map.codes.fa-relationship` |
| "Office to IBD mapping" | `yhat.map.office.ibd-relationship` |
| "Custodian validation rule" | `yhat.rule.custodian.validation` |
| "FA↔Office mapping per institution" | `yhat.map.fa-office.{institution}` |
| "Duplicate detection logic" | `yhat.obs.aum.duplicate-detection` |
| "Code creation process" | `yhat.process.codes.creation` |
| "Auth token expiry" | `yhat.excep.auth.token-expiry` |

## Engram Schema (Verified Fields)

### Native mem_save Fields

| Field | Value | Notes |
|-------|-------|-------|
| `project` | `yhat` | Fixed project |
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
- captured_at: {ISO timestamp}
- review_after: {ISO timestamp}
- evidence: {table.field, file, ticket reference}
- related_topic: {topic_key|null}
- supersedes: {topic_key|null}
```

## Capture Protocol

### Step 1: Initial Check (On Activation)

```
1. Check if Engram tools are available
2. mem_search({ project: "yhat", query: "yhat", type: "yhat-knowledge", limit: 50 })
3. Summarize: existing domains, empty areas, pending questions count
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

### Step 5: Silent Save

```javascript
// Immediately after user states a fact
mem_save({
  project: "yhat",
  type: "yhat-knowledge",
  topic_key: "yhat.map.fa-office.banco-nacion",
  title: "FA to Office mapping for Banco Nación",
  content: `FA codes map to Office 'OF-BNA-001' for Banco Nación custodian.

## YHat Metadata
- source: business-user
- original_type: mapping
- confidence: 0.95
- status: proposed
- validated_by: pending
- captured_at: ${new Date().toISOString()}
- review_after: ${futureDate(7)}
- evidence: "CustodianMapping table, OfficeCode column"
- related_topic: null`,
  scope: "project"
})

// One-line confirmation only
"Guardado: yhat.map.fa-office.banco-nacion"
```

### Step 6: Evaluate Enrichment Gaps

For each saved fact, evaluate:
- Business rationale (why this rule exists)
- Scope and exceptions
- Origin (which system/table/field)
- Validity period
- Owner who can validate
- Real example
- Relation to other records

### Step 7: Auto-Enrichment

Before enqueuing a question, try to answer from session context:

```javascript
// If user shared a SQL query with the mapping
if (userSharedQuery.includes("OfficeCode")) {
  // Update the existing record
  mem_update({
    id: existingRecordId,
    content: existingContent + "\n- evidence: \"SQL query: SELECT * FROM CustodianMapping WHERE OfficeCode LIKE 'BNA%'\""
  })
  // Do NOT mark as confirmed
}
```

### Step 8: Queue Management

If cannot auto-enrich, save question as:

```javascript
mem_save({
  project: "yhat",
  type: "yhat-knowledge",
  topic_key: "yhat.assum.fa-office.banco-nacion.scope",
  title: "Scope of Banco Nación FA mapping",
  content: `Question: Does this FA↔Office mapping apply to IBD system as well?

Related to: yhat.map.fa-office.banco-nacion

## YHat Metadata
- source: ai-agent
- original_type: assumption
- confidence: 0.4
- status: pending-enrichment
- validated_by: pending
- captured_at: ${new Date().toISOString()}
- review_after: ${futureDate(3)}`,
  scope: "project"
})
```

## When to Surface Questions

### Natural Pause
User says: "listo", "gracias", "eso es todo", "terminamos", or changes topic clearly.

```
Tengo ${queue.length} preguntas para completar lo que capturé.
¿Las vemos ahora o las dejo para después?
```

### Explicit Request
User says: "enriquecé", "qué te falta", "hacéme preguntas", "show queue"

```
${formatQueue(queue)}
```

### Real Blockage
Ambiguity would make the record incorrect AND cannot continue.

```
Solo una pregunta sobre esto: [brief question]
```

### Session Close
See Session Close Protocol below.

## Queue Format

```
Tengo ${n} preguntas para cerrar lo que capturé:

1. [yhat.rule.codes.primary-key] ¿Cuál es el rango de fechas de vigencia?
2. [yhat.map.fa-office] ¿Este mapeo aplica también para IBD?
3. [yhat.assum.api-response] ¿Hay documentación oficial?

Respondé "skip" para las que no puedas responder ahora.
```

## Session Close Protocol

```
Al cerrar la sesión:

1. Listar hechos durables sin guardar → ofrecer guardarlos
2. Mostrar cola: "Tengo ${n} preguntas pendientes. ¿Las respondemos ahora?"
3. Si el usuario dice "después" → no insistir en esta sesión
4. La próxima sesión: mencionar cola pendiente en resumen inicial
```

## Audit Mode

Activated with: "auditá yhat", "yhat audit", "reporte de yhat"

### Report Format

```
## YHat Audit Report — ${date}

### Coverage
- Total records: ${total}
- By type: decision(${n}), rule(${n}), map(${n}), obs(${n}), assum(${n})
- By domain: fa(${n}), codes(${n}), office(${n}), custodian(${n})

### Pending Review
- status: proposed → ${n}
- status: pending-enrichment → ${n}
- review_after < today → ${n}

### Quality
- confidence < 0.7 → ${n}
- Sin evidence → ${n}

### Enrichment Queue
- Preguntas abiertas → ${n}
- Sin respuesta por 14+ días → ${n}

### Coverage Gaps
- Custodians sin conocimiento: ${list}
- Instituciones sin mapeos: ${list}

### Top 10 Preguntas
1. [pregunta] → [topic_key]
...
```

## Validation Checklist

Before calling mem_save:

- [ ] `project` is `yhat`
- [ ] `type` is `yhat-knowledge`
- [ ] `topic_key` matches `yhat\.[a-z]+\.[a-z]+.*`
- [ ] `title` is short and descriptive
- [ ] `content` includes knowledge + YHat Metadata section
- [ ] `scope` is `project`
- [ ] No secrets/credentials in any field
- [ ] No session-specific paths
- [ ] One atomic fact per record (not multiple rules in one)

## Example Payloads

### Valid: Business Rule

```json
{
  "project": "yhat",
  "type": "yhat-knowledge",
  "topic_key": "yhat.rule.codes.primary-key",
  "title": "Code uses composite key",
  "content": "Code entities are identified by a composite key of (CodeId, SourceSystem).\n\n## YHat Metadata\n- source: business-user\n- original_type: decision\n- confidence: 0.95\n- status: proposed\n- validated_by: pending\n- captured_at: 2025-01-15T11:00:00Z\n- review_after: 2025-01-22T11:00:00Z\n- evidence: \"CodeEntity.cs, composite key definition\"",
  "scope": "project"
}
```

### Valid: Mapping

```json
{
  "project": "yhat",
  "type": "yhat-knowledge",
  "topic_key": "yhat.map.fa-office.banco-nacion",
  "title": "FA to Office mapping for Banco Nación",
  "content": "FA codes map to Office 'OF-BNA-001' for Banco Nación custodian.\n\n## YHat Metadata\n- source: business-user\n- original_type: mapping\n- confidence: 0.9\n- status: proposed\n- validated_by: pending\n- captured_at: 2025-01-15T10:00:00Z\n- review_after: 2025-01-22T10:00:00Z\n- evidence: \"CustodianMapping table, OfficeCode column\"",
  "scope": "project"
}
```

### Valid: Assumption with Enrichment Question

```json
{
  "project": "yhat",
  "type": "yhat-knowledge",
  "topic_key": "yhat.assum.fa-office.banco-nacion.ibd-scope",
  "title": "IBD scope for Banco Nación mapping",
  "content": "Question: Does this FA↔Office mapping apply to IBD system?\n\nRelated to: yhat.map.fa-office.banco-nacion\n\n## YHat Metadata\n- source: ai-agent\n- original_type: assumption\n- confidence: 0.4\n- status: pending-enrichment\n- validated_by: pending\n- captured_at: 2025-01-15T10:05:00Z\n- review_after: 2025-01-18T10:05:00Z",
  "scope": "project"
}
```

### Rejected: Secret

```json
❌ { "content": "API key is stored as env var API_KEY=sk-123..." }
```

### Rejected: Session Summary

```json
❌ { "content": "Session summary: discussed FA display, reviewed code..." }
```

## Version

1.1.0 — Silent capture, enrichment queue, audit mode
