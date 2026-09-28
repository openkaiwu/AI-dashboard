# AI Hub Flutter client

Android / Windows / Linux source for the AI Hub mobile client.
Drift contains profiles, sync_state, pending_ops, notes and conflicts.
System session tokens are stored separately in flutter_secure_storage.

The Android UI has Codex and Cursor mode selection, quota overview, planning,
reminders, reset radar, rule controls, manual quota entry, device and Bridge
status, offline notes, and server profiles. Codex news shows the last check
status, freshness, and verified source links. Collection itself runs on the
bound desktop Bridge; the phone receives server snapshots. Mobile notifications
are currently foreground in-app prompts, not remote background push.

~~~bash
flutter pub get
flutter analyze
flutter test
AIHUB_INTEGRATION_URL=http://127.0.0.1:8080 flutter test test/real_server_test.dart
~~~

Real-server tests use loopback HTTP and create disposable integration accounts.
Production Android rejects cleartext traffic. Use a valid HTTPS Server Profile.

For a local Android debug connection, run `adb reverse tcp:8080 tcp:8080`
before using `http://127.0.0.1:8080` as the server profile. Debug builds allow
loopback HTTP for this case; release builds require HTTPS for remote servers.

Android build requires Java/Android SDK. Windows build requires Windows Flutter/Visual Studio C++.
Release signing is not configured; the local release APK uses the Android debug
key and is not a store-ready artifact. See ../../docs/VALIDATION.md for the
executed checks and device-level limits.
