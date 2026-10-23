import 'dart:async';

/// Foreground reminders must not wait for note/data synchronization.
class ForegroundReminderPoller {
  ForegroundReminderPoller({
    required this.check,
    required this.onError,
    this.interval = const Duration(seconds: 15),
  });

  final Future<void> Function() check;
  final void Function(Object) onError;
  final Duration interval;
  Timer? _timer;
  bool _active = false;
  bool _disposed = false;
  Future<void>? _inFlight;

  void setActive(bool active) {
    if (_disposed || active == _active) return;
    _active = active;
    _timer?.cancel();
    if (active) {
      unawaited(checkNow());
      _timer = Timer.periodic(interval, (_) => unawaited(checkNow()));
    }
  }

  Future<void> checkNow() {
    if (_disposed || !_active) return Future.value();
    return _inFlight ??= _run();
  }

  Future<void> _run() async {
    // Defer even a synchronous callback failure until _inFlight is assigned.
    await Future<void>.value();
    try {
      if (_disposed || !_active) return;
      await check();
    } catch (e) {
      if (!_disposed) onError(e);
    } finally {
      _inFlight = null;
    }
  }

  void dispose() {
    _disposed = true;
    _active = false;
    _timer?.cancel();
  }
}
