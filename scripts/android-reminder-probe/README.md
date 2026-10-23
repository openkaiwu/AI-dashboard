# Android reminder dispatcher regression probe

Run only on a disposable Android emulator, with the app's **debug APK** installed
and notification permission granted. The instrumentation APK must use the same
debug signing key. It calls the real `BackgroundSync` class through the target
class loader and intercepts HTTPS inside that test process. No real server,
login, account or push gateway is used.

Build under WSL with JDK 17 and Android SDK build-tools 36.0.0 / android-35:

```sh
bash scripts/android-reminder-probe/build.sh
```

Install `.runtime/android-reminder-probe/probe.apk`, then run each mode in a fresh
instrumentation process:

```sh
adb -s emulator-5554 shell am instrument -w -e mode normal dev.aihub.reminderqa/qa.ReminderProbe
```

Modes:

| Mode | Assertion |
| --- | --- |
| `normal` | Cache failures do not prevent system submission; second check does not reserve or post again. |
| `quiet` | Quiet hours prevent reservation. |
| `denied` | With notification permission revoked by the test setup, no event is consumed and the permission failure remains visible. |
| `release_retry` | A definitely unsent reservation whose release fails is durably released on the next check, then submitted once. |
| `join` | Two concurrent checks wait for one in-flight delivery and both complete. |
| `auth` | 401 pauses delivery; lifecycle rescheduling cannot restart an unauthorized job. |
| `foreground` | A resumed debug-only Activity displays the real native dialog; no duplicate system notification or second reservation. |
| `foreground_quiet` | A resumed Activity still respects quiet hours and reserves nothing. |
| `foreground_paused` | A finished Activity receives no dialog; delivery falls back to a system notification. |
| `foreground_preview` | Same assertions as foreground, holds the synthetic dialog for 45 seconds for visual inspection. |
| `channel_race` | Channel closes during reservation: no submission/seen marker; server reservation is released. Run **last**: this deliberately disables the emulator's reminder channel. |

Preferences are restored after each test; synthetic notification records may
remain in the emulator. `auth` cancels the test target's scheduled job, and
`channel_race` leaves the test channel disabled. These are reasons not to run this
on a user's phone. A pass proves native dispatch behavior, **not** real push
transport, background OS scheduling latency, or visual heads-up presentation.
