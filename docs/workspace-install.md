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

---

## MSI Installer (Per-User, Silent)

For non-developer operators, the MSI installer provides a double-click install
without requiring `go install` or a terminal.

### Installing from the MSI

1. **Download** the `.msi` from the GitHub release page (or the IT-shared location).
2. **Double-click** `yhat-agent-<version>-amd64.msi`.
3. The installer runs silently — no dialog, no UAC prompt.
4. When it finishes, `yhat-agent` is installed at:

   ```
   %LOCALAPPDATA%\Programs\YHat Agent\yhat-agent.exe
   ```

5. Restart OpenCode and Claude Desktop to pick up the new MCP registration.

> **SmartScreen warning:** The MSI is unsigned. On first run, Windows
> SmartScreen may show "Windows protected your PC → More info → Run anyway".
> Click "Run anyway". Contact IT to add a publisher exception once a
> code-signing cert is issued (R1 from PRD).

### Verifying the MSI Installation

After installing, run these commands in PowerShell to confirm the install
succeeded:

```powershell
# 1. Binary exists at the correct location
Get-Item "$env:LOCALAPPDATA\Programs\YHat Agent\yhat-agent.exe"

# 2. Version matches the release
& "$env:LOCALAPPDATA\Programs\YHat Agent\yhat-agent.exe" version

# 3. Self-test passes (F0 regression)
& "$env:LOCALAPPDATA\Programs\YHat Agent\yhat-agent.exe" mcp --selftest
# Expected: {"ok":true,"fts5":true,"mcp":true,...}

# 4. Install path stored in registry (for uninstall)
Get-ItemProperty HKCU:\Software\YHat Agent -Name InstallPath

# 5. Status shows Cerebro section (F1-C)
& "$env:LOCALAPPDATA\Programs\YHat Agent\yhat-agent.exe" status
# Expected: Cerebro section with YHat home, DB path, Memories count
```

To verify the MCP server appears in OpenCode and Claude Desktop:

```powershell
# OpenCode: check for yhat in opencode.json
Get-Content "$env:APPDATA\.config\opencode\opencode.json" |
  ConvertFrom-Json | Select-Object -ExpandProperty mcp

# Claude Desktop: check for yhat in claude_desktop_config.json
Get-Content "$env:APPDATA\Claude\claude_desktop_config.json" |
  ConvertFrom-Json | Select-Object -ExpandProperty mcpServers
```

### Uninstalling the MSI

```powershell
# Via the MSI (from the original .msi file)
msiexec /x yhat-agent-<version>-amd64.msi

# Or via Add/Remove Programs
Start → Settings → Apps → YHat Agent → Uninstall
```

The uninstall custom action runs `yhat-agent uninstall --yes` before removing
files, which cleans up:

- F0 OpenCode + Claude Desktop MCP registrations
- F1 Cerebro `config.yaml`, `state.json`, `yhat.db`

The registry key `HKCU\Software\YHat Agent` is removed automatically by MSI.

### Honest Limits of MSI Verification on This Box

The MSI cannot be built or tested end-to-end on the Linux development
environment because WiX is a Windows-only toolchain. Automated smoke tests
in CI verify the `.msi` table structure using `msiinfo`. Manual verification
on a real Windows WorkSpace is required before release (see F1-F runbook in
`odd/tasks/cerebro-f1f.md`).
