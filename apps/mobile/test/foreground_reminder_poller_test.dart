import 'dart:async';
import 'package:flutter_test/flutter_test.dart';
import 'package:aihub_mobile/foreground_reminder_poller.dart';

void main() {
  testWidgets('reminders continue while unrelated data sync is pending',
      (tester) async {
    final dataSync = Completer<void>();
    var checks = 0;
    final poller = ForegroundReminderPoller(
      check: () async {
        checks++;
      },
      onError: (_) {},
    );
    poller.setActive(true);
    await tester.pump();
    await tester.pump(const Duration(seconds: 15));
    expect(dataSync.isCompleted, isFalse);
    expect(checks, 2);
    dataSync.complete();
    poller.dispose();
  });

  testWidgets('overlapping ticks join, failed check retries next tick',
      (tester) async {
    final pending = Completer<void>();
    var checks = 0;
    final errors = <Object>[];
    final poller = ForegroundReminderPoller(
        check: () {
          checks++;
          return checks == 1 ? pending.future : Future.value();
        },
        onError: errors.add);
    poller.setActive(true);
    await tester.pump();
    await tester.pump(const Duration(seconds: 45));
    expect(checks, 1);
    pending.completeError(StateError('offline'));
    await tester.pump();
    expect(errors, hasLength(1));
    await tester.pump(const Duration(seconds: 15));
    expect(checks, 2);
    poller.dispose();
  });

  testWidgets(
      'pause stops polling, resume checks immediately, dispose stops it',
      (tester) async {
    var checks = 0;
    final poller = ForegroundReminderPoller(
        check: () async {
          checks++;
        },
        onError: (_) {});
    poller.setActive(true);
    await tester.pump();
    poller.setActive(false);
    await tester.pump(const Duration(minutes: 2));
    expect(checks, 1);
    poller.setActive(true);
    await tester.pump();
    expect(checks, 2);
    poller.dispose();
    await tester.pump(const Duration(minutes: 2));
    await poller.checkNow();
    expect(checks, 2);
  });

  testWidgets('synchronous exception does not permanently occupy checking slot',
      (tester) async {
    var checks = 0;
    final poller = ForegroundReminderPoller(
        check: () {
          checks++;
          throw StateError('check failed');
        },
        onError: (_) {});
    poller.setActive(true);
    await tester.pump();
    await tester.pump(const Duration(seconds: 15));
    expect(checks, 2);
    poller.dispose();
  });
}
