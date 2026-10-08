# yhat-agent MCP F0 Prototype

**Experimental F0** — synthetic data only, ephemeral in-memory.

## Commands

```bash
# Run MCP stdio server (connects to an MCP client)
yhat-agent mcp

# Self-test: verifies MCP stack, exits 0 on success, JSON report to stdout
yhat-agent mcp --selftest
```

## Tool: fts5_search

Single read-only full-text search tool using SQLite FTS5 (unicode61 remove_diacritics).

| | |
|---|---|
| Input | `{"query": string}` (max 200 chars) |
| Matching | accent- and case-insensitive |
| Result | `{"hits":[{"title","text","rank"}],"total_found","query"}` |

**Fixtures:** three synthetic entries (`EXAMPLE-NOT-REAL`, `Política Example`, `Café Example`). No real knowledge, no persistence.

## Windows Claude Desktop

```json
{
  "mcpServers": {
    "yhat-f0": {
      "command": "C:\\absolute\\path\\to\\yhat-agent.exe",
      "args": ["mcp"]
    }
  }
}
```

> Configure via `%APPDATA%\Claude\claude_desktop_config.json`. The server name
> is `yhat-agent-f0`. Requires real Windows testing before production use.
> Rename and security policy checks are pending (see F0-3).

## Build

```bash
PATH=/snap/go/current/bin:$PATH CGO_ENABLED=0 go build -o yhat-agent ./cmd/yhat-agent
```

## Test

```bash
PATH=/snap/go/current/bin:$PATH CGO_ENABLED=0 go test ./internal/spike ./cmd/yhat-agent
```
