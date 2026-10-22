import 'store.dart';

class ReminderPolicy {
  static String alertState(Json a, DateTime now) {
    if (a['dismissed'] == true) return 'dismissed';
    if (a['read_at'] != null) return 'read';
    final snooze = DateTime.tryParse(a['snoozed_until']?.toString() ?? '');
    if (snooze != null && snooze.isAfter(now)) return 'snoozed';
    return 'pending';
  }

  static bool eligibleAdvisor(Json a, DateTime now) {
    final advice = a['advice'] as Json?;
    return alertState(a, now) == 'pending' &&
        DateTime.tryParse(a['notified_at']?.toString() ?? '') != null &&
        advice?['interrupt'] != false &&
        ['slow', 'credit'].contains(advice?['kind']);
  }

  static bool eligibleGeneric(Json n) {
    final key = n['dedupe_key']?.toString() ?? '';
    return n['active'] != false &&
        n['status'] == 'unread' &&
        ['warning', 'critical'].contains(n['severity']) &&
        !(n['provider_slug'] == 'codex' && key.startsWith('quota.')) &&
        !key.startsWith('quota.reset_soon_unused:');
  }

  static List<Json> candidates(
      Json overview, List<Json> notifications, DateTime now) {
    final generated =
        DateTime.tryParse(overview['generated_at']?.toString() ?? '');
    if (overview['quiet'] == true ||
        generated == null ||
        now.difference(generated) > const Duration(minutes: 1) ||
        generated.difference(now) > const Duration(minutes: 2)) {
      return [];
    }
    final result = <Json>[];
    if ((overview['preferences'] as Json?)?['enabled'] == true) {
      for (final a in List<Json>.from(overview['alerts'] as List? ?? [])) {
        if (!eligibleAdvisor(a, now)) continue;
        final advice = a['advice'] as Json;
        result.add({
          'id': 'codex:${Uri.encodeComponent(a['id'] as String)}',
          'title': advice['source_name'] == null
              ? advice['title']
              : '${advice['source_name']} · ${advice['title']}',
          'body': advice['body'],
          'severity': (advice['priority'] as num) >= 4 ? 'critical' : 'warning',
          'provider_slug': 'codex',
          'provider_name': 'Codex',
          'created_at': a['first_seen'],
          '_token': a['notified_at'],
          '_priority': advice['priority'],
        });
      }
    }
    for (final n in notifications) {
      if (!eligibleGeneric(n)) continue;
      result.add({
        ...n,
        '_token': n['delivery_token'] ?? n['id'],
        '_priority': n['severity'] == 'critical' ? 4 : 2
      });
    }
    result.sort((a, b) {
      final priority = (b['_priority'] as num).compareTo(a['_priority'] as num);
      return priority != 0
          ? priority
          : (a['id'] as String).compareTo(b['id'] as String);
    });
    return result;
  }

  static bool canInterrupt(Json? last, num priority, DateTime now) {
    final at = last?['at'], previous = last?['priority'];
    if (at is! num ||
        previous is! num ||
        !at.isFinite ||
        !previous.isFinite ||
        at > now.millisecondsSinceEpoch + 120000) {
      return true;
    }
    return now.millisecondsSinceEpoch - at >= 1800000;
  }

  static String remainingLabel(num value) {
    if (!value.isFinite) return '未知';
    if (value > 0 && value < 0.1) return '<0.1';
    return value > 0 && value < 1
        ? value.toStringAsFixed(1)
        : value.toStringAsFixed(0);
  }
}
