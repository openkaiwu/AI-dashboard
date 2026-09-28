#!/usr/bin/env bash
set -euo pipefail
ROOT="$1"
SDK="$2"
export ANDROID_HOME="$SDK"
export ANDROID_SDK_ROOT="$SDK"
if [ -z "${JAVA_HOME:-}" ] && [ -d /usr/lib/jvm/java-17-openjdk-amd64 ]; then
  export JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64
fi
if [ -n "${JAVA_HOME:-}" ]; then
  export PATH="$JAVA_HOME/bin:$PATH"
fi
export PATH="$ANDROID_HOME/platform-tools:$PATH"
FLUTTER_BIN="${FLUTTER_BIN:-/opt/aihub-tools/flutter/bin/flutter}"
cd "$ROOT/apps/mobile"
"$FLUTTER_BIN" pub get
"$FLUTTER_BIN" build apk --release
