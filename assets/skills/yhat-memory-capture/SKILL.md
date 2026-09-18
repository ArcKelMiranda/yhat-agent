---
name: yhat-memory-capture
description: "Capture durable YHat knowledge to Engram. Triggered when the yhat-memory-capture agent is selected. Saves to Engram via mem_save with yhat.* topic pattern."
compatibility: opencode
version: "1.0.0"
---

# YHat Memory Capture Skill

Convert durable YHat knowledge emerged during conversation into Engram observations with the `yhat.*` topic pattern.

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
- Or any meaningful domain from the content

### Topic Key Examples

| Content Topic | Topic Key |
|--------------|-----------|
| "FA display separator" | `yhat.rule.fa.display-separator` |
| "Code to FA relationship" | `yhat.map.codes.fa-relationship` |
| "Branch mapping rules" | `yhat.rule.branch.mapping` |
| "Duplicate detection logic" | `yhat.obs.aum.duplicate-detection` |
| "Code creation process" | `yhat.process.codes.creation` |
| "Auth token expiry" | `yhat.excep.auth.token-expiry` |

## Save Contract

### Required mem_save Fields

| Field | Value | Notes |
|-------|-------|-------|
| `project` | `yhat` | Fixed project |
| `type` | `yhat-knowledge` | All YHat observations |
| `topic_key` | `yhat.{type}.{domain}.{concept}` | Pattern: 3-4 segments |
| `title` | Short, descriptive | 1 sentence max |
| `content` | Knowledge + metadata | See format below |
| `scope` | `project` | Fixed scope |
| `review_after` | ISO timestamp | +7 days default |
| `source_session` | Session ID | From runtime |
| `source_workspace` | Workspace path | From runtime |

### Content Format

```
{knowledge statement in natural language}

## YHat Metadata
- source: {business-user|ai-agent|validation-agent}
- original_type: {vocabulary type}
- confidence: {0.0-1.0}
- captured_at: {ISO timestamp}
```

### Optional Fields

| Field | Default | Notes |
|-------|---------|-------|
| `source_tool` | null | Tool that captured |
| `created_by` | null | Anonymized identifier |
| `related_topic` | null | For cross-references |

## Vocabulary → Topic Mapping

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

## Eligibility Rules

### MUST Capture

✅ Business rules and decisions
✅ Verified facts from operational systems
✅ Technical standards and conventions
✅ Entity relationships and mappings
✅ Process descriptions
✅ Exception handling rules
✅ Data quality observations

### MUST Reject

❌ **Secrets**: passwords, API keys, tokens, bearer, ENV vars
❌ **Transient**: session summaries, transcripts, debug logs
❌ **Drafts**: WIP, TODO, TBD, unconfirmed guesses (frame as `assum` if explicit)
❌ **Personal Data**: user paths, local configs, session state
❌ **Chatty Content**: greetings, acknowledgements, off-topic

## Capture Protocol

### Step 1: Identify Knowledge
Stay attentive throughout the conversation for durable knowledge opportunities.

### Step 2: Validate Eligibility
- Check against rejection patterns (secrets, transient, drafts, personal data)
- Reject ineligible content silently

### Step 3: Map Vocabulary
- Extract keywords from content
- Map to appropriate topic segment

### Step 4: Build Topic Key
- Pattern: `yhat.{segment}.{domain}.{concept}`
- Lowercase, hyphens for spaces
- 3-4 segments total

### Step 5: Format Content
- Knowledge statement first
- YHat Metadata section at bottom
- Include source, original_type, confidence, timestamp

### Step 6: Call mem_save

```javascript
mem_save({
  project: "yhat",
  type: "yhat-knowledge",
  topic_key: "yhat.rule.fa.display-separator",
  title: "FA display uses space-slash-space",
  content: `When a Code has multiple FA values, they are displayed separated by ' / ' (space-slash-space).

## YHat Metadata
- source: ai-agent
- original_type: business-rule
- confidence: 0.9
- captured_at: ${new Date().toISOString()}`,
  scope: "project",
  review_after: "${futureDate(7)}",
  source_session: "${sessionId}",
  source_workspace: "${workspace}"
})
```

### Step 7: Review Reminder
At session wrap-up, ask:
> "¿Hay algo importante que capture durante esta sesión que deba registrarse?"

## Rejection Patterns

### Secrets (Always Reject)

```
- password, passwd, pwd
- secret, api_key, apikey, api-key
- token, bearer, credentials
- aws_secret, private_key
- ENV vars with secrets
```

### Transient (Always Reject)

```
- summary, transcript, session-log
- conversation-log, debug-output
- "last message", "continuing from"
- session state
```

### Drafts (Always Reject)

```
- draft, wip, TODO, TBD
- "[ ]", "check this later"
- "not sure yet", "might change"
```

### Personal Data (Always Reject)

```
- /home/username/...
- Session-specific variables
- Machine configs
- Temporary debug vars
```

## Query Examples

### Retrieve All YHat Knowledge

```javascript
// All YHat observations
mem_search({ project: "yhat", topic_key: "yhat.*" })

// Only decisions
mem_search({ project: "yhat", topic_key: "yhat.decision.*" })

// Only business rules
mem_search({ project: "yhat", topic_key: "yhat.rule.*" })

// FA related
mem_search({ project: "yhat", topic_key: "yhat.*.fa.*" })

// Pending review (needs_review state)
mem_review({ action: "list", project: "yhat" })
```

## Example Payloads

### Valid: Business Rule

```json
{
  "project": "yhat",
  "type": "yhat-knowledge",
  "topic_key": "yhat.rule.fa.display-separator",
  "title": "FA display uses space-slash-space",
  "content": "When a Code has multiple FA values, they are displayed separated by ' / ' (space-slash-space).\n\n## YHat Metadata\n- source: ai-agent\n- original_type: business-rule\n- confidence: 0.9\n- captured_at: 2025-01-15T10:30:00Z",
  "scope": "project",
  "review_after": "2025-01-22T10:30:00Z",
  "source_session": "sess_abc123",
  "source_workspace": "/workspace/my-project"
}
```

### Valid: Decision

```json
{
  "project": "yhat",
  "type": "yhat-knowledge",
  "topic_key": "yhat.decision.codes.primary-key",
  "title": "Code uses composite key",
  "content": "Code entities are identified by a composite key of (CodeId, SourceSystem).\n\n## YHat Metadata\n- source: business-user\n- original_type: decision\n- confidence: 0.95\n- captured_at: 2025-01-15T11:00:00Z",
  "scope": "project",
  "review_after": "2025-01-22T11:00:00Z",
  "source_session": "sess_def456",
  "source_workspace": "/workspace/my-project"
}
```

### Valid: Assumption

```json
{
  "project": "yhat",
  "type": "yhat-knowledge",
  "topic_key": "yhat.assum.fa.sorting",
  "title": "FA values should be sorted",
  "content": "I assume FA values should be sorted alphabetically for consistency, but this needs verification.\n\n## YHat Metadata\n- source: ai-agent\n- original_type: assumption\n- confidence: 0.6\n- captured_at: 2025-01-15T11:30:00Z",
  "scope": "project",
  "review_after": "2025-01-16T11:30:00Z",
  "source_session": "sess_ghi789",
  "source_workspace": "/workspace/my-project"
}
```

### Rejected: Transient Summary

```
❌ "Session summary: We discussed FA display, reviewed the code, and decided to use space-slash"
```

### Rejected: Secret

```
❌ "The API key is stored as environment variable API_KEY=secret123xyz"
```

### Rejected: Draft

```
❌ "TODO: verify this behavior with the actual data"
```

## Validation Checklist

Before calling mem_save:

- [ ] `project` is `yhat`
- [ ] `type` is `yhat-knowledge`
- [ ] `topic_key` matches `yhat\.[a-z]+\.[a-z]+.*`
- [ ] `title` is short and descriptive
- [ ] `content` includes knowledge + YHat Metadata
- [ ] `scope` is `project`
- [ ] `review_after` is set (default +7 days)
- [ ] No secrets/credentials in any field
- [ ] No session-specific paths
- [ ] `source_session` and `source_workspace` captured

## Notes

- **Human review required**: All observations start pending review
- **Engram storage**: Uses `mem_save` — no separate YHat backend needed
- **Filtering**: Use `topic_key: "yhat.*"` to retrieve all YHat knowledge
- **Single system**: YHat and regular project memories coexist in Engram
- **Confidence defaults**: 0.7 for normal, higher with strong evidence, lower for assumptions
