#!/usr/bin/env bash
# build-msi.sh — Build the yhat-agent MSI installer using WiX 3.14.
#
# Usage:
#   ./build-msi.sh [--version VERSION] [--source EXE_PATH] [--output DIR]
#
# Environment:
#   YHAT_VERSION   Override version string (default: 3.0.0.0)
#   YHAT_SOURCE    Path to yhat-agent.exe (default: dist/yhat-agent_windows_amd64.exe)
#   YHAT_OUTPUT    Output directory (default: dist)
#
# Requirements:
#   - Windows with WiX 3.14 toolset (candle.exe, light.exe) in PATH, OR
#   - Linux/macOS with Wine + WiX 3.14 (WiX is downloaded to $HOME/.wix/ if missing)
#
#   To download WiX 3.14 on Linux:
#     curl -L https://github.com/wixtoolset/wix3/releases/download/wix3142rtm/wix314-binaries.zip \
#       -o /tmp/wix.zip && unzip -o /tmp/wix.zip -d "$HOME/.wix" && rm /tmp/wix.zip
#
#   To use with Wine (Linux):
#     WINE=wine        # or path to wine binary
#     WIXDIR=$HOME/.wix # directory containing candle.exe / light.exe
#
# Exit codes:
#   0  MSI built successfully
#   1  WiX not found and auto-download failed
#   2  candle.exe failed
#   3  light.exe failed
#   4  Unsupported platform (no WiX, no Wine)

set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────
VERSION="${YHAT_VERSION:-3.0.0.0}"
SOURCE="${YHAT_SOURCE:-dist/yhat-agent_windows_amd64.exe}"
OUTPUT="${YHAT_OUTPUT:-dist}"
WIX_VERSION="3.14.0.7012.20180122"   # WiX 3.14 stable tag
WIX_BASEURL="https://github.com/wixtoolset/wix3/releases/download/wix3142rtm"
WIX_ZIP="wix314-binaries.zip"
WIX_DIR="${WIX_DIR:-$HOME/.wix}"

# ── Helpers ───────────────────────────────────────────────────────────────────
usage() {
  cat <<EOF
Usage: $0 [options]
  --version VERSION  MSI ProductVersion (default: $VERSION)
  --source  EXE      Path to yhat-agent.exe (default: $SOURCE)
  --output  DIR      Output directory (default: $OUTPUT)
  --help             Show this message
EOF
  exit 0
}

info()  { echo "[build-msi] $*"; }
warn()  { echo "[build-msi] WARNING: $*" >&2; }
error() { echo "[build-msi] ERROR: $*" >&2; }

need_cmd() {
  if ! command -v "$1" &>/dev/null; then
    error "$1 not found in PATH"
    return 1
  fi
}

# ── Argument parsing ───────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --help)    usage ;;
    --version) VERSION="$2"; shift 2 ;;
    --source)  SOURCE="$2";  shift 2 ;;
    --output)  OUTPUT="$2";  shift 2 ;;
    *)         error "Unknown option: $1"; usage ;;
  esac
done

# ── Locate WiX ─────────────────────────────────────────────────────────────────
# Priority: PATH > WIX_DIR > auto-download
locate_wix() {
  local wix_candle=""
  local wix_light=""

  # 1. Check PATH
  if need_cmd candle.exe 2>/dev/null && need_cmd light.exe 2>/dev/null; then
    info "Found WiX in PATH"
    return 0
  fi

  # 2. Check WIX_DIR
  if [[ -x "$WIX_DIR/candle.exe" ]] && [[ -x "$WIX_DIR/light.exe" ]]; then
    info "Found WiX in WIX_DIR=$WIX_DIR"
    export PATH="$WIX_DIR:$PATH"
    return 0
  fi

  # 3. Auto-download (Linux only, only if --version ≥ 3.14 is requested)
  if [[ "$(uname)" == "Linux" ]]; then
    info "WiX not found — downloading to $WIX_DIR"
    local zip_path="/tmp/wix.zip"
    if [[ ! -f "$zip_path" ]]; then
      info "Downloading WiX 3.14 from GitHub (≈ 28 MB)..."
      if ! curl -fsSL --retry 3 --retry-delay 5 \
         -o "$zip_path" \
         "${WIX_BASEURL}/${WIX_ZIP}"; then
        error "Failed to download WiX. Check your network connection."
        error "Alternatively, download manually from:"
        error "  ${WIX_BASEURL}/${WIX_ZIP}"
        error "and extract to $WIX_DIR"
        return 1
      fi
    fi
    mkdir -p "$WIX_DIR"
    if ! unzip -o "$zip_path" -d "$WIX_DIR" > /dev/null 2>&1; then
      error "Failed to extract WiX zip."
      return 1
    fi
    if [[ ! -x "$WIX_DIR/candle.exe" ]]; then
      error "candle.exe not found after extraction. WiX layout may have changed."
      return 1
    fi
    info "WiX extracted to $WIX_DIR"
    export PATH="$WIX_DIR:$PATH"
    return 0
  fi

  # 4. Wine attempt (Linux with Wine installed)
  if [[ "$(uname)" == "Linux" ]] && command -v wine &>/dev/null; then
    local wine_wix="$HOME/.wine/drive_c/wix"
    if [[ -x "$wine_wix/candle.exe" ]] && [[ -x "$wine_wix/light.exe" ]]; then
      info "Found WiX in Wine prefix at $wine_wix"
      export PATH="$wine_wix:$PATH"
      return 0
    fi
    warn "Wine found but WiX not in Wine prefix. Install WiX to:"
    warn "  $wine_wix"
    warn "Or run on Windows."
    return 1
  fi

  # No WiX available
  return 1
}

# ── Pre-flight checks ─────────────────────────────────────────────────────────
check_source() {
  if [[ ! -f "$SOURCE" ]]; then
    error "Source binary not found: $SOURCE"
    error "Build the binary first:"
    error "  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \\"
    error "    go build -ldflags=\"-s -w\" -o dist/yhat-agent_windows_amd64.exe \\"
    error "    ./cmd/yhat-agent"
    return 1
  fi
  info "Source: $SOURCE ($(wc -c < "$SOURCE" | tr -d ' ') bytes)"
}

# ── Build ─────────────────────────────────────────────────────────────────────
build() {
  local wxs="installer/yhat-agent.wxs"
  local wixobj="${OUTPUT}/yhat-agent.wixobj"
  local msi="${OUTPUT}/yhat-agent-${VERSION}-amd64.msi"

  if [[ ! -f "$wxs" ]]; then
    error "WiX source not found: $wxs"
    return 1
  fi

  mkdir -p "$OUTPUT"

  info "Step 1/2 — candle.exe (compile WiX source)"
  # candle.exe resolves YHAT_VERSION and YHAT_SOURCE_DIR as defines.
  # Use Windows-style absolute path for the source so MSI stores it correctly.
  local src_abs
  src_abs="$(cd "$(dirname "$SOURCE")" && pwd)/$(basename "$SOURCE")"
  src_abs="${src_abs//\//\\}"   # convert to backslash for Windows
  candle.exe \
    -d"YHAT_VERSION=${VERSION}" \
    -d"YHAT_SOURCE_DIR=${src_abs}" \
    -out "$wixobj" \
    "$wxs" 2>&1
  local candle_status=$?
  if [[ $candle_status -ne 0 ]]; then
    error "candle.exe exited with status $candle_status"
    return 2
  fi
  info "  → $wixobj"

  info "Step 2/2 — light.exe (link MSI)"
  # -ext WixUIExtension is needed for UIRef to resolve (silent but harmless without UI).
  light.exe \
    -ext WixUIExtension \
    -out "$msi" \
    "$wixobj" 2>&1
  local light_status=$?
  if [[ $light_status -ne 0 ]]; then
    error "light.exe exited with status $light_status"
    return 3
  fi
  info "  → $msi"

  info "MSI built successfully: $msi ($(wc -c < "$msi" | tr -d ' ') bytes)"
  return 0
}

# ── Main ───────────────────────────────────────────────────────────────────────
main() {
  info "Building yhat-agent MSI v${VERSION}"
  info "Source binary: $SOURCE"
  info "Output dir:    $OUTPUT"

  if [[ ! -f "installer/yhat-agent.wxs" ]]; then
    error "Must be run from the repository root (or with correct paths)."
    exit 1
  fi

  if ! locate_wix; then
    error ""
    error "WiX 3.14 toolset not found."
    error ""
    error "To install on Windows:"
    error "  1. Download: https://github.com/wixtoolset/wix3/releases/download/wix3142rtm/wix314-binaries.zip"
    error "  2. Extract to C:\\wix (or any directory)"
    error "  3. Add that directory to your PATH"
    error ""
    error "To install on Linux (with Wine):"
    error "  1. Install Wine: sudo apt install wine (or your distro's package)"
    error "  2. mkdir -p \$HOME/.wix"
    error "  3. curl -L ... | unzip - to \$HOME/.wix"
    error "  4. Set: WIXDIR=\$HOME/.wix ./installer/build-msi.sh"
    error ""
    error "To auto-download WiX on Linux (no Wine, for cross-compile only):"
    error "  ./installer/build-msi.sh"
    error "(The script downloads WiX to \$HOME/.wix if running on Linux.)"
    exit 1
  fi

  if ! check_source; then
    exit 1
  fi

  if ! build; then
    exit $?
  fi

  info "Done."
}

main "$@"
