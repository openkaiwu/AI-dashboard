# AI Hub Flutter client

Android / Windows / Linux source for the M0 local-first client.
Drift contains profiles, sync_state, pending_ops, notes and conflicts.
System session tokens are stored separately in flutter_secure_storage.

~~~bash
flutter pub get
flutter analyze
flutter test
AIHUB_INTEGRATION_URL=http://127.0.0.1:8080 flutter test test/real_server_test.dart
~~~

Real-server tests use loopback HTTP and create disposable integration accounts.
Production Android rejects cleartext traffic. Use a valid HTTPS Server Profile.

Android build requires Java/Android SDK. Windows build requires Windows Flutter/Visual Studio C++.
Release signing is not configured. These native builds and device-level UI checks were not performed here;
see ../../docs/VALIDATION.md for the exact executed scope.
