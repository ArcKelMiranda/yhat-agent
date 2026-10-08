# Windows WorkSpace MCP Registration

## Overview

On Windows WorkSpaces, `yhat-agent install` also registers the MCP server with OpenCode and Claude Desktop. This enables the diagnostic F0 prototype (`yhat-agent mcp`) to be available as an MCP tool in those editors without requiring manual configuration.

## Quick Path

```powershell
# Install the agent (requires go install or manual binary placement)
go install github.com/ArcKelMiranda/yhat-agent@latest

# Run the installer
yhat-agent install

# Restart OpenCode or Claude Desktop to pick up the new MCP configuration
```

## Details

| Topic | Decision |
|-------|----------|
| Platform | CLI guard enforces Windows-only; library is platform-neutral |
| When | MCP registration runs automatically during `yhat-agent install` on Windows only; no-op elsewhere |
| Binary | Uses `os.Executable()` absolute path; no PATH changes or binary relocation |
| OpenCode config | `%APPDATA%\.config\opencode\opencode.json` (or custom via `OPENCODE_CONFIG`) |
| Claude config | `%APPDATA%\Claude\claude_desktop_config.json` or `%LOCALAPPDATA%\Packages\Claude_*\LocalCache\Roaming\Claude\claude_desktop_config.json` |
| Schema (OpenCode) | `mcp.yhat = { type: "local", command: [exe, "mcp"], enabled: true }` |
| Schema (Claude) | `mcpServers.yhat = { command: exe, args: ["mcp"] }` |
| Conflict handling | Conflicting `yhat` entry (different command, extra fields, changed enabled flag) is an error; identical entry is a no-op |
| Backup | Unique byte-copy backup created alongside config before any modification |
| Idempotence | Re-running install with identical registration is a no-op with no backup |

## MCP Configuration Paths

OpenCode looks for `opencode.json` in the OpenCode config root. You can override this with:

```powershell
$env:OPENCODE_CONFIG = "D:\my-config\opencode.json"
yhat-agent install
```

Claude Desktop uses the first found directory from:

1. Conventional: `%APPDATA%\Claude\claude_desktop_config.json`
2. Packaged: `%LOCALAPPDATA%\Packages\Claude_abc123\LocalCache\Roaming\Claude\claude_desktop_config.json`

If multiple Claude installations are found, installation fails with an explicit error.

## Configuration Requirements

**JSON-only.** Configuration files must be valid JSON. JSONC (JSON with comments, trailing commas) is not supported. Any config file containing `//` or `/* */` comments, or whose content cannot be parsed as strict JSON, is rejected.

- OpenCode: If `opencode.jsonc` exists alongside `opencode.json`, registration is refused
- Config files containing duplicate keys at any nesting level are rejected
- Empty config files are treated as invalid (not absent)
- Root-level `null`, arrays, or scalars are rejected

## Failure and Backup Behavior

If MCP registration fails:

- The original config file is **never modified**
- A backup of the original is created alongside the config (if the file existed)
- `yhat-agent install` exits with code **1** and prints a diagnostic message
- Legacy asset installation results are **not printed** when registration fails (atomic registration-or-exit)

**New config creation** (absent target) uses exclusive file creation (`O_EXCL`): only one concurrent caller can succeed; others receive "refusing to overwrite". No backup is created for new files.

**Existing config update** (file present) uses temp-file + rename: a backup is created before modification, and the write is rejected if the file changes between the initial read and the commit.

Example backup name: `opencode.json.opencode.backup.<timestamp>.json`

To recover: restore from the backup file and resolve the conflict.

## After Installation

1. **Close** OpenCode and Claude Desktop completely
2. **Reopen** the application
3. The MCP server `yhat` should appear in the MCP tools list

## Limitations

- MCP registration is **separate** from legacy asset install/uninstall
- On non-Windows, `yhat-agent install` completes asset installation but performs no MCP registration (no-op)
- If MCP registration fails, legacy assets are installed but `install` exits with code 1 (partial outcome)
- Uninstalling with `yhat-agent uninstall --yes` removes managed assets but **does not** remove MCP configuration entries
- To remove MCP registration, manually edit the config files and remove the `yhat` entry from `mcp` (OpenCode) or `mcpServers` (Claude)

## Troubleshooting

**"yhat already registered"** — The entry is identical; no changes made.

**"conflicting yhat entry"** — An existing `yhat` MCP entry differs from the desired registration (different command, extra environment fields, or changed enabled flag). Remove it manually before re-running install.

**"multiple Claude installations found"** — Uninstall one Claude installation or specify which to use.

**"OPENCODE_CONFIG must be absolute"** — Set `OPENCODE_CONFIG` to a full path, e.g. `C:\Users\you\opencode.json`.

**"config contains duplicate keys"** — Your config file has duplicate keys at some nesting level. Remove the duplicates and re-run.

**"opencode.jsonc found alongside opencode.json"** — OpenCode refuses ambiguous configurations. Remove the `.jsonc` file or rename your config.

**"MCP registration failed" exit code 1** — MCP server registration encountered an error (duplicate keys, symlink target, external modification during write, or ambiguous Claude installation). Legacy assets may still be installed. Check the specific per-client error message above.

## Verification

```powershell
# Self-test (runs MCP server diagnostics)
yhat-agent mcp --selftest
```
