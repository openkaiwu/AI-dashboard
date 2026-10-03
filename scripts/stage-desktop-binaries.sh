#!/usr/bin/env bash
# Stage local-server binaries into apps/desktop/resources/bin for packaging on
# Linux or macOS. Windows keeps scripts/Stage-Desktop-Binaries.ps1.
# Usage: stage-desktop-binaries.sh [linux|darwin] [amd64|arm64]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PLATFORM="${1:-${AIHUB_PLATFORM:-linux}}"
ARCH="${2:-${AIHUB_ARCH:-}}"
case "$PLATFORM" in
  linux)  ARCH="${ARCH:-amd64}" ;;
  darwin) ARCH="${ARCH:-arm64}" ;;
  *) echo "unsupported platform: $PLATFORM (use linux or darwin)" >&2; exit 1 ;;
esac
SRC="$ROOT/artifacts/aihub-m0"
BIN="$ROOT/apps/desktop/resources/bin"
for f in "aihub-$PLATFORM-$ARCH" "aihub-bridge-$PLATFORM-$ARCH"; do
  [[ -f "$SRC/$f" ]] || { echo "Missing $SRC/$f. Run scripts/build.sh first." >&2; exit 1; }
done
mkdir -p "$BIN"
# Only the current platform's binaries may ship inside the installer.
rm -f "$BIN/aihub-server" "$BIN/aihub-server.exe" "$BIN/aihub-bridge" "$BIN/aihub-bridge.exe"
install -m 0755 "$SRC/aihub-$PLATFORM-$ARCH" "$BIN/aihub-server"
install -m 0755 "$SRC/aihub-bridge-$PLATFORM-$ARCH" "$BIN/aihub-bridge"
echo "Staged $PLATFORM/$ARCH embedded services in $BIN"
