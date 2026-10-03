import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:aihub_mobile/strong_reminder_ui.dart';

void main() {
  testWidgets('strong reminder modal blocks dismiss and offers actions',
      (tester) async {
    String? action;
    final notification = {
      'id': 'ntf_test',
      'provider_slug': 'codex',
      'provider_name': 'Codex',
      'title': '多用提醒',
      'body': '套餐：Pro · 隐藏五小时额度及提醒\n剩余额度：35%（提醒线 35%）\n临近重置：24 小时内',
      'severity': 'warning',
      'created_at': '2026-09-30T13:00:00Z',
    };

    await tester.pumpWidget(MaterialApp(
      home: Builder(
        builder: (context) => Scaffold(
          body: ElevatedButton(
            onPressed: () async {
              action = await showDialog<String>(
                context: context,
                barrierDismissible: false,
                builder: (ctx) =>
                    StrongReminderDialog(notification: notification),
              );
            },
            child: const Text('trigger'),
          ),
        ),
      ),
    ));

    await tester.tap(find.text('trigger'));
    await tester.pumpAndSettle();

    expect(find.text('Codex · 多用提醒'), findsOneWidget);
    expect(find.text('剩余额度：35%（提醒线 35%）'), findsOneWidget);
    expect(find.text('强提醒'), findsOneWidget);
    expect(find.text('已读'), findsOneWidget);
    expect(find.text('1 小时后提醒'), findsOneWidget);
    expect(find.text('忽略本次'), findsOneWidget);

    await tester.tap(find.text('已读'));
    await tester.pumpAndSettle();
    expect(action, 'read');
  });

  test('warning severity is eligible for popup polling', () {
    final items = [
      {'status': 'read', 'severity': 'warning', 'id': 'a'},
      {'status': 'unread', 'severity': 'info', 'id': 'b'},
      {'status': 'unread', 'severity': 'warning', 'id': 'c'},
    ];
    final next = items.cast<Map<String, dynamic>?>().firstWhere(
          (n) =>
              n != null &&
              n['status'] == 'unread' &&
              (n['severity'] == 'warning' || n['severity'] == 'critical'),
          orElse: () => null,
        );
    expect(next?['id'], 'c');
  });

  test('notification title uses severity badge', () {
    final title = StrongReminderCopy.notificationTitle({
      'provider_name': 'Codex',
      'title': '临近重置',
      'severity': 'warning',
    });
    expect(title, '[强提醒] Codex · 临近重置');
  });
}
