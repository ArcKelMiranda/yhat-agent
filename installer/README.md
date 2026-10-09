# yhat-agent MSI Installer

## Overview

This directory contains the WiX 3.14 source (`yhat-agent.wxs`) and build
scripts for producing a per-user MSI installer for yhat-agent.

**The MSI is unsigned.** Until a code-signing certificate is available,
Windows SmartScreen or the WorkSpace policy may show a warning. The
operator can approve the exception by clicking "Run anyway" or by
contacting IT to whitelist the publisher `YHat` once the cert is issued.

## Files

| File | Purpose |
|------|---------|
| `yhat-agent.wxs` | WiX 3.14 source — product metadata, component, custom actions |
| `build-msi.sh` | POSIX bash build script (cross-platform, auto-downloads WiX on Linux) |
| `installer.cmd` | Windows `.cmd` convenience script (runs on Windows cmd.exe) |
| `README.md` | This file |

## Build

### On Windows

**Option A — PowerShell (recommended):**

```powershell
# Build the Go binary first (if not already built)
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build `
    -ldflags="-s -w" `
    -o dist/yhat-agent_windows_amd64.exe `
    ./cmd/yhat-agent

# Build the MSI
$VERSION = "3.0.1.0"
$src = "dist\yhat-agent_windows_amd64.exe"
candle.exe -dYHAT_VERSION=$VERSION -dYHAT_SOURCE_DIR=$src installer\yhat-agent.wxs
light.exe -ext WixUIExtension dist\yhat-agent.wixobj -o dist\yhat-agent-$VERSION-amd64.msi
```

**Option B — Use the convenience script:**

```cmd
installer.cmd 3.0.1.0
```

### On Linux (cross-compile with Wine)

```bash
# WiX is a Windows tool. On Linux, the script downloads it and runs under Wine.
./installer/build-msi.sh --version 3.0.1.0 --source dist/yhat-agent_windows_amd64.exe

# The script auto-downloads WiX to $HOME/.wix if not found.
```

> **Note:** Wine is required on Linux. Install it with:
> `sudo apt install wine` (Debian/Ubuntu) or your distro's package.

### On macOS

WiX does not run natively on macOS. Use a Windows VM, Docker with Wine,
or build on a Windows machine.

## Custom Actions

The MSI ships two custom actions:

| CustomAction | Phase | When | Command |
|---|---|---|---|
| `CA_Install_CMD` | Immediate | Fresh install | Sets `YHAT_INSTALL_CMD` property |
| `CA_Install` | Deferred | Fresh install | `yhat-agent.exe install --no-bandeja` |
| `CA_ReadUninstPath` | Immediate | Uninstall/upgrade | Sets `YHAT_UNINSTALL_CMD` from registry |
| `CA_Uninstall` | Immediate | Uninstall/upgrade | `yhat-agent.exe uninstall --yes` |

### Install path registry key

The MSI stores the install path in `HKCU\Software\YHat Agent\InstallPath`
at install time. The uninstall custom action reads this key to locate
`yhat-agent.exe` even after the MSI is no longer on disk.

### Exit codes

- `CA_Install` propagates `yhat-agent install`'s exit code (1 on error).
- `CA_Uninstall` propagates `yhat-agent uninstall`'s exit code.
- `msiexec` inherits the non-zero code and exits accordingly.

## Version Strategy

The MSI `ProductVersion` uses the 4-octet format required by MSI
(`3.0.0.0` for v0.3.0-f1). CI overrides this via the `YHAT_VERSION`
environment variable:

```bash
candle.exe -dYHAT_VERSION=3.0.1.0 ...
```

The `UpgradeCode` (`7e3a8c4d-...`) is fixed so every version upgrades
the same package.

## Per-User Install

`ALLUSERS=""` and `InstallScope="perUser"` mean:

- No admin rights required — the MSI runs from a normal user account.
- Files land in `%LOCALAPPDATA%\Programs\YHat Agent\`.
- Registry keys land in `HKCU` (current user's hive).
- No UAC prompt.

## Uninstall

```powershell
# Via the MSI
msiexec /x yhat-agent-3.0.1.0-amd64.msi

# Via Add/Remove Programs
# Start → Settings → Apps → YHat Agent → Uninstall

# Via yhat-agent directly (if the binary is still present)
& "%LOCALAPPDATA%\Programs\YHat Agent\yhat-agent.exe" uninstall --yes
```

The uninstall custom action runs `yhat-agent uninstall --yes` before
`RemoveExistingProducts` removes the files. This ensures the F0
OpenCode + Claude registration and F1 Cerebro state are cleaned up
correctly.

## Code Signing (R1 from PRD)

The MSI ships **unsigned**. Until a code-signing certificate is
acquired:

1. Windows SmartScreen may show a warning on first run.
2. The WorkSpace policy may block `.msi` files from unknown publishers.
3. Operators should click **"Run anyway"** in SmartScreen, or contact IT
   to add a policy exception for `YHat` as the publisher name.

When a code-signing cert is available, add these steps to the CI
workflow (see `.github/workflows/yhat-agent-release.yml`):

1. Sign the `.msi` with `signtool sign /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 $MSI`
2. Verify the signature with `signtool verify /pa /v $MSI`

The signtool is available in the Windows SDK or Visual Studio.

## Manual Verification (Windows)

After installing the MSI:

```powershell
# 1. Check the binary exists
Get-Item "$env:LOCALAPPDATA\Programs\YHat Agent\yhat-agent.exe"

# 2. Run the self-test
& "$env:LOCALAPPDATA\Programs\YHat Agent\yhat-agent.exe" mcp --selftest
# Expected: {"ok":true,...}

# 3. Check version
& "$env:LOCALAPPDATA\Programs\YHat Agent\yhat-agent.exe" version

# 4. Check the registry key (for uninstall path)
Get-ItemProperty HKCU:\Software\YHat Agent -Name InstallPath

# 5. Check Add/Remove Programs entry
Get-ItemProperty HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*
  # or for per-user:
Get-ItemProperty HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*
```

To inspect the MSI without installing (on Linux or Windows with lessmsi/msiinfo):

```bash
# On Linux (with msiinfo from msitools, if available):
msiinfo database dist/yhat-agent-3.0.1.0-amd64.msi

# On Windows:
msiexec /a dist/yhat-agent-3.0.1.0-amd64.msi     # administrative install (extracts to chosen folder)
msiinfo export dist/yhat-agent-3.0.1.0-amd64.msi # dump tables
```

## Honest Limits

- MSI building requires a Windows toolchain. Automated CI validation
  runs in GitHub Actions on a `windows-latest` runner.
- End-to-end install/uninstall testing requires a real Windows VM
  (not available in Linux CI). Smoke tests verify the `.msi` table
  structure with `msiinfo`.
- No code signing — see the Code Signing section above.
- No AutoUpdate feed — updates come from new MSI releases or
  `yhat-agent update`.

## Related Documentation

- `docs/workspace-install.md` — End-user install guide for the WorkSpace
- `.github/workflows/yhat-agent-release.yml` — CI build + release workflow
