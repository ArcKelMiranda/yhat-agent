# yhat-agent

Installable Go CLI for the YHat knowledge capture agent and skill.

## What it does

`yhat-agent` is a self-contained binary that installs the YHat OpenCode agent and skill assets into your local OpenCode configuration directory. It tracks only its own files and can safely update from GitHub Releases.

YHat captures durable operational knowledge (business rules, decisions, patterns) during AI coding sessions and saves it to **Engram** with a structured topic pattern for easy filtering.

## Installation

```bash
go install github.com/ArcKelMiranda/yhat-agent/cmd/yhat-agent@latest
```

Or download pre-built binaries from the [Releases page](https://github.com/ArcKelMiranda/yhat-agent/releases).

## Usage

```bash
yhat-agent install   # Install embedded agent and skill assets
yhat-agent status    # Report installation state without making changes
yhat-agent update    # Download and verify latest release from GitHub
yhat-agent uninstall --yes   # Remove managed files (requires confirmation)
```

## YHat Knowledge Capture

When the YHat agent is selected for a session, it identifies durable knowledge from conversation and saves it to Engram.

### Topic Pattern

All YHat observations use: `yhat.{type}.{domain}.{concept}`

| Type | Segment | Example |
|------|---------|---------|
| Decision | `decision` | `yhat.decision.fa-display` |
| Business Rule | `rule` | `yhat.rule.codes.primary-key` |
| Observation | `obs` | `yhat.obs.duplicate-detection` |
| Assumption | `assum` | `yhat.assum.api-response-format` |
| Process | `process` | `yhat.process.code-creation` |
| Mapping | `map` | `yhat.map.code-fa-relationship` |
| Exception | `excep` | `yhat.excep.auth-token-expired` |
| Definition | `def` | `yhat.def.code-entity` |
| Integration | `intg` | `yhat.intg.payment-gateway` |
| Data Anomaly | `anom` | `yhat.anom.missing-values` |
| Technical Rule | `tech` | `yhat.tech.retry-backoff` |

### Filtering YHat Knowledge

```javascript
// All YHat observations
mem_search({ project: "yhat", topic_key: "yhat.*" })

// Only decisions
mem_search({ project: "yhat", topic_key: "yhat.decision.*" })

// FA related rules
mem_search({ project: "yhat", topic_key: "yhat.rule.fa.*" })
```

### Benefits

- **Single system**: Uses Engram — no separate YHat backend
- **Easy filtering**: Topic pattern `yhat.*` retrieves all YHat knowledge
- **Human review**: All observations start pending review (`review_after`)
- **Shared storage**: Engram.db can be shared with other projects that read `yhat.*`

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
└── *_test.go                # Test suite
```

## License

MIT
