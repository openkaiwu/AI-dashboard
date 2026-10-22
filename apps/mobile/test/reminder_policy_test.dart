import 'package:flutter_test/flutter_test.dart';
import 'package:aihub_mobile/reminder_policy.dart';

void main() {
  final now = DateTime.utc(2026, 10, 8, 4);
  final alert = <String, dynamic>{
    'id': 'window:1',
    'notified_at': now.toIso8601String(),
    'dismissed': false,
    'snoozed_until': null,
    'advice': {'kind': 'slow', 'priority': 4, 'title': '额度已用完', 'body': '等待重置'},
  };
  test('only unread quota risk and expiry can interrupt', () {
    expect(ReminderPolicy.eligibleAdvisor(alert, now), isTrue);
    expect(
        ReminderPolicy.eligibleAdvisor({
          ...alert,
          'advice': {'kind': 'slow', 'interrupt': false}
        }, now),
        isFalse);
    expect(
        ReminderPolicy.eligibleAdvisor(
            {...alert, 'read_at': now.toIso8601String()}, now),
        isFalse);
    expect(ReminderPolicy.eligibleAdvisor({...alert, 'notified_at': null}, now),
        isFalse);
    for (final kind in ['use', 'news']) {
      expect(
          ReminderPolicy.eligibleAdvisor({
            ...alert,
            'advice': {'kind': kind}
          }, now),
          isFalse);
    }
    expect(
        ReminderPolicy.eligibleGeneric({
          'status': 'unread',
          'severity': 'warning',
          'provider_slug': 'codex',
          'dedupe_key': 'quota.low:1',
        }),
        isFalse);
    expect(
        ReminderPolicy.eligibleGeneric({
          'status': 'unread',
          'severity': 'critical',
          'active': false,
        }),
        isFalse);
  });
  test('quiet hours and stale responses suppress both channels', () {
    final overview = <String, dynamic>{
      'generated_at': now.toIso8601String(),
      'quiet': false,
      'preferences': {'enabled': true},
      'alerts': [alert],
    };
    expect(ReminderPolicy.candidates(overview, [], now).single['id'],
        'codex:window%3A1');
    expect(ReminderPolicy.candidates({...overview, 'quiet': true}, [], now),
        isEmpty);
    expect(
        ReminderPolicy.candidates(
            overview, [], now.add(const Duration(minutes: 2))),
        isEmpty);
    expect(
        ReminderPolicy.candidates({
          ...overview,
          'preferences': {'enabled': false}
        }, [], now),
        isEmpty);
  });
  test('hard 30 minute cadence cannot be bypassed by priority',
      () {
    final last = {'at': now.millisecondsSinceEpoch, 'priority': 3};
    expect(ReminderPolicy.canInterrupt(last, 3, now), isFalse);
    expect(ReminderPolicy.canInterrupt(last, 4, now), isFalse);
    expect(
        ReminderPolicy.canInterrupt(
            last, 2, now.add(const Duration(minutes: 30))),
        isTrue);
    expect(ReminderPolicy.remainingLabel(0.04), '<0.1');
    expect(ReminderPolicy.remainingLabel(0.3), '0.3');
  });
}
