import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:aihub_mobile/appearance.dart';
import 'package:aihub_mobile/theme.dart';
import 'package:aihub_mobile/strong_reminder_ui.dart';

void main() {
  test('restores preference and serializes rapid writes', () async {
    String? stored = 'dark';
    final first = Completer<void>();
    var calls = 0;
    final controller = AppearanceController(
        read: () async => stored,
        write: (value) async {
          if (calls++ == 0) await first.future;
          stored = value;
        });
    await controller.load();
    expect(controller.mode, ThemeMode.dark);
    final a = controller.select(ThemeMode.light);
    final b = controller.select(ThemeMode.system);
    expect(controller.mode, ThemeMode.system);
    first.complete();
    expect(await a, isTrue);
    expect(await b, isTrue);
    final restored =
        AppearanceController(read: () async => stored, write: (_) async {});
    await restored.load();
    expect(restored.mode, ThemeMode.system);
    controller.dispose();
    restored.dispose();
  });
  test('late load cannot undo a selection and write failure is visible',
      () async {
    final old = Completer<String?>();
    final controller = AppearanceController(
        read: () => old.future,
        write: (_) async => throw StateError('storage unavailable'));
    final load = controller.load();
    expect(await controller.select(ThemeMode.dark), isFalse);
    old.complete('light');
    await load;
    expect(controller.mode, ThemeMode.dark);
    controller.dispose();
    expect(parseAppearance('corrupt'), ThemeMode.system);
  });
  testWidgets('dark reminders retain readable foreground and semantic actions',
      (tester) async {
    await tester.pumpWidget(MaterialApp(
        theme: HubTheme.dark(),
        home: Scaffold(
            body: StrongReminderDialog(notification: {
          'title': '额度提醒',
          'body': '额度即将重置\n--- 数据截至 10:30',
          'severity': 'warning',
          'provider_slug': 'codex',
        }))));
    expect(find.text('Codex · 额度提醒'), findsOneWidget);
    expect(find.text('1 小时后提醒'), findsOneWidget);
    final context = tester.element(find.byType(StrongReminderDialog));
    final colors = HubTheme.colors(context);
    final ratio = (colors.muted.computeLuminance() + .05) /
        (colors.surface.computeLuminance() + .05);
    expect(ratio, greaterThanOrEqualTo(4.5));
    expect(tester.takeException(), isNull);
  });
}
