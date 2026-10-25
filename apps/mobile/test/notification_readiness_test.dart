import 'package:aihub_mobile/notification_readiness.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('channel off never appears ready even when app permission is granted',
      () {
    final state = notificationReadiness(
        {'notifications': true, 'notification_state': 'channel_blocked'});
    expect(state.ready, false);
    expect(state.label, '额度通知已关闭');
    expect(state.hint, contains('应用内提醒也不会弹出'));
  });
  test('notification permission denial takes priority over channel state', () {
    final state = notificationReadiness(
        {'notifications': false, 'notification_state': 'heads_up_conditions'});
    expect(state.ready, false);
    expect(state.label, '通知未允许');
  });
  test('system do not disturb remains visible as a blocker', () {
    final state = notificationReadiness(
        {'notifications': true, 'notification_state': 'system_dnd'});
    expect(state.ready, false);
    expect(state.label, '系统免打扰中');
  });
  test('tray-only permission does not claim a banner is ready', () {
    final state = notificationReadiness(
        {'notifications': true, 'notification_state': 'tray_only'});
    expect(state.ready, true);
    expect(state.label, '通知栏已允许');
  });
}
