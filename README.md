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

YHat captures durable operational knowledge (business rules, decisions, patterns) during
AI coding sessions and saves it to **Engram** with a structured topic pattern for easy
filtering.

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
├── *_test.go                # Test suite
└── go.sum                   # Go module checksums
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
