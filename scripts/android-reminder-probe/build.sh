#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SDK="${ANDROID_SDK_ROOT:-$ROOT/.tools/android-sdk}"
BT="$SDK/build-tools/36.0.0"
JAR="$SDK/platforms/android-35/android.jar"
OUT="$ROOT/.runtime/android-reminder-probe"
mkdir -p "$OUT/classes" "$OUT/dex"
javac -encoding UTF-8 -source 17 -target 17 -cp "$JAR" -d "$OUT/classes" "$ROOT/scripts/android-reminder-probe/src/qa/ReminderProbe.java"
"$BT/d8" --lib "$JAR" --min-api 26 --output "$OUT/dex" "$OUT"/classes/qa/*.class
"$BT/aapt2" link --manifest "$ROOT/scripts/android-reminder-probe/AndroidManifest.xml" -I "$JAR" -o "$OUT/unsigned.apk"
python3 - "$OUT" <<'PY'
import sys, zipfile
from pathlib import Path
p=Path(sys.argv[1])
with zipfile.ZipFile(p/'unsigned.apk','a') as z: z.write(p/'dex/classes.dex','classes.dex')
PY
"$BT/zipalign" -f 4 "$OUT/unsigned.apk" "$OUT/aligned.apk"
"$BT/apksigner" sign --ks "${HOME}/.android/debug.keystore" --ks-pass pass:android --key-pass pass:android --out "$OUT/probe.apk" "$OUT/aligned.apk"
