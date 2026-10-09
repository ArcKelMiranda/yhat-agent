@echo off
:: installer.cmd — Build the yhat-agent MSI installer on Windows.
::
:: Usage:
::   installer.cmd [VERSION [SOURCE_EXE]]
::
:: Defaults:
::   VERSION   = 3.0.0.0  (override with first argument or set YHAT_VERSION env var)
::   SOURCE    = dist\yhat-agent_windows_amd64.exe
::               (override with second argument or set YHAT_SOURCE env var)
::
:: Requirements:
::   - WiX 3.14 toolset in PATH
::     Download from: https://github.com/wixtoolset/wix3/releases/download/wix3142rtm/wix314-binaries.zip
::     Extract to C:\wix (or any directory on PATH)
::
::   - Git Bash or PowerShell (this script runs in cmd.exe natively)
::
:: Exit codes:
::   0  MSI built successfully
::   1  candle.exe failed
::   2  light.exe failed
::   3  Source binary not found
::   4  WiX not found

setlocal enabledelayedexpansion

set "VERSION=%~1"
if not defined VERSION set "VERSION=%YHAT_VERSION%"
if not defined VERSION set "VERSION=3.0.0.0"

set "SOURCE=%~2"
if not defined SOURCE set "SOURCE=%YHAT_SOURCE%"
if not defined SOURCE set "SOURCE=dist\yhat-agent_windows_amd64.exe"

set "OUTPUT=dist"
set "WXS=installer\yhat-agent.wxs"
set "WIXOBJ=%OUTPUT%\yhat-agent.wixobj"
set "MSI=%OUTPUT%\yhat-agent-%VERSION%-amd64.msi"

echo [installer.cmd] Building yhat-agent MSI v%VERSION%
echo [installer.cmd] Source:  %SOURCE%
echo [installer.cmd] Output:  %MSI%

:: ── Pre-flight ───────────────────────────────────────────────────────────────

where candle.exe >nul 2>&1
if errorlevel 1 (
    echo [installer.cmd] ERROR: candle.exe not found in PATH.
    echo.
    echo Download WiX 3.14 from:
    echo   https://github.com/wixtoolset/wix3/releases/download/wix3142rtm/wix314-binaries.zip
    echo.
    echo Extract to C:\wix (or any directory on PATH) and re-run this script.
    exit /b 4
)

where light.exe >nul 2>&1
if errorlevel 1 (
    echo [installer.cmd] ERROR: light.exe not found in PATH.
    echo.
    echo Download WiX 3.14 from:
    echo   https://github.com/wixtoolset/wix3/releases/download/wix3142rtm/wix314-binaries.zip
    echo.
    echo Extract to C:\wix (or any directory on PATH) and re-run this script.
    exit /b 4
)

if not exist "%SOURCE%" (
    echo [installer.cmd] ERROR: Source binary not found: %SOURCE%
    echo.
    echo Build the binary first:
    echo   CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist\yhat-agent_windows_amd64.exe .\cmd\yhat-agent
    exit /b 3
)

if not exist "%WXS%" (
    echo [installer.cmd] ERROR: WiX source not found: %WXS%
    exit /b 1
)

:: ── Build ─────────────────────────────────────────────────────────────────────

if not exist "%OUTPUT%" mkdir "%OUTPUT%"

:: Resolve absolute path for the source (WiX requires an absolute path).
for %%i in ("%SOURCE%") do set "SRC_ABS=%%~fi"
set "SRC_ABS=%SRC_ABS:\=\\%"

echo [installer.cmd] Step 1/2 — candle.exe (compile WiX source)
candle.exe ^
    -d"YHAT_VERSION=%VERSION%" ^
    -d"YHAT_SOURCE_DIR=%SRC_ABS%" ^
    -out "%WIXOBJ%" ^
    "%WXS%"
if errorlevel 1 (
    echo [installer.cmd] ERROR: candle.exe failed.
    exit /b 1
)
echo [installer.cmd]   -> %WIXOBJ%

echo [installer.cmd] Step 2/2 — light.exe (link MSI)
light.exe ^
    -ext WixUIExtension ^
    -out "%MSI%" ^
    "%WIXOBJ%"
if errorlevel 1 (
    echo [installer.cmd] ERROR: light.exe failed.
    exit /b 2
)
echo [installer.cmd]   -> %MSI%

echo [installer.cmd] MSI built successfully.
echo [installer.cmd] Size: %~z1 bytes (approx)
for %%F in ("%MSI%") do echo [installer.cmd] Size: %%~zF bytes

exit /b 0
