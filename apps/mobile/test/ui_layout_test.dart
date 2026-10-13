import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;
import 'package:aihub_mobile/api.dart';
import 'package:aihub_mobile/codex_page.dart';
import 'package:aihub_mobile/store.dart';
import 'package:aihub_mobile/theme.dart';
import 'package:aihub_mobile/workspace_chrome.dart';
import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  for (final plan in ['pro', 'plus']) {
    for (final width in [360.0, 390.0]) {
      testWidgets(
          'Codex $plan quota visibility and layout at $width logical pixels',
          (tester) async {
        tester.view.devicePixelRatio = 1;
        tester.view.physicalSize = Size(width, 844);
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        const fontPath = String.fromEnvironment('UI_FONT');
        if (fontPath.isNotEmpty) {
          final loader = FontLoader('UiPreview');
          loader.addFont(Future.value(
              ByteData.sublistView(File(fontPath).readAsBytesSync())));
          await loader.load();
          final icons = FontLoader('MaterialIcons');
          icons.addFont(rootBundle.load('fonts/MaterialIcons-Regular.otf'));
          await icons.load();
        }
        final now = DateTime.now();
        final fixture = {
          'preferences': {
            'enabled': true,
            'near_hours': 12,
            'spare_percent': 50,
            'low_percent': 15,
            'pace_lead': 20,
            'credit_hours': 48,
            'quiet_start': 23,
            'quiet_end': 8,
            'utc_offset_minutes': 480
          },
          'plan': {'plan_type': plan, 'source_type': 'manual'},
          'alerts': [],
          'quiet': false,
          'devices': [
            {
              'id': 'preview-computer',
              'name': '我的工作电脑',
              'snapshot': {
                'status': 'ok',
                'observed_at': now.toIso8601String(),
                'reset_credits': {'available_count': 2, 'items': []},
                // Exercise five-hour data reported in the second slot.
                'buckets': [
                  {
                    'id': 'codex',
                    'primary': {
                      'duration_minutes': 10080,
                      'used_percent': 14.0
                    },
                    'secondary': {'duration_minutes': 300, 'used_percent': 28.0}
                  }
                ]
              },
              'analysis': {
                'state': 'balanced',
                'advice': [],
                'metrics': [
                  {
                    'bucket': 'codex',
                    'slot': 'primary',
                    'duration_minutes': 300,
                    'remaining': 72.0,
                    'hours_left': 3.5,
                    'daily_budget': 49.4,
                    'rate_per_hour': 5.2,
                    'resets_at': now.millisecondsSinceEpoch ~/ 1000 + 12600
                  },
                  {
                    'bucket': 'codex',
                    'slot': 'secondary',
                    'duration_minutes': 10080,
                    'remaining': 86.0,
                    'hours_left': 124.0,
                    'daily_budget': 16.6,
                    'rate_per_hour': 1.1,
                    'resets_at': now.millisecondsSinceEpoch ~/ 1000 + 446400
                  },
                ]
              },
            }
          ],
        };
        final store = HubStore(NativeDatabase.memory());
        final api = HubApi('http://localhost', (_) async {},
            client: MockClient((request) async => http.Response(
                jsonEncode(request.url.path.endsWith('/overview')
                    ? fixture
                    : request.url.path.endsWith('/consumption')
                        ? {
                            'series': [
                              for (var i = 0; i < 7; i++)
                                {
                                  'bucket_start': now
                                      .toUtc()
                                      .subtract(Duration(days: 6 - i))
                                      .toIso8601String()
                                      .substring(0, 10),
                                  'consumed_pp': i == 3 ? null : 1.2 + i * 0.6,
                                  'coverage_hours': i == 3 ? 0 : 24
                                }
                            ],
                            'hourly': [],
                            'generations': [
                              {
                                'resets_at':
                                    now.millisecondsSinceEpoch ~/ 1000 + 12600,
                                'points': [
                                  {'t': now.toIso8601String(), 'used': 28.0}
                                ]
                              }
                            ]
                          }
                        : <String, dynamic>{}),
                200,
                headers: {'content-type': 'application/json; charset=utf-8'})));
        final boundary = GlobalKey();
        var destination = 0;
        final baseTheme = HubTheme.light();
        await tester.pumpWidget(RepaintBoundary(
            key: boundary,
            child: MaterialApp(
              debugShowCheckedModeBanner: false,
              theme: fontPath.isEmpty
                  ? baseTheme
                  : baseTheme.copyWith(
                      textTheme:
                          baseTheme.textTheme.apply(fontFamily: 'UiPreview')),
              home: StatefulBuilder(
                  builder: (context, setState) => Scaffold(
                        appBar: AppBar(
                            title: const HubBrand(subtitle: 'Codex 工作空间')),
                        body: Padding(
                            padding: const EdgeInsets.fromLTRB(20, 16, 20, 0),
                            child: CodexPage(
                                api: api,
                                store: store,
                                scope: 'preview-layout')),
                        bottomNavigationBar: HubNavigationBar(
                            selectedIndex: destination,
                            onSelected: (value) =>
                                setState(() => destination = value)),
                      )),
            )));
        // Database and HTTP fixture work must finish before inspecting the layout.
        await tester.runAsync(() async {
          await Future<void>.delayed(const Duration(milliseconds: 300));
        });
        await tester.pumpAndSettle();
        expect(find.text('节奏平稳'), findsOneWidget);
        expect(find.text('5 小时额度'),
            plan == 'plus' ? findsOneWidget : findsNothing);
        expect(find.text('7 天额度'), findsOneWidget);
        expect(tester.takeException(), isNull);
        const output = String.fromEnvironment('UI_SCREENSHOT_DIR');
        if (output.isNotEmpty) {
          final render = boundary.currentContext!.findRenderObject()!
              as RenderRepaintBoundary;
          await tester.runAsync(() async {
            final image = await render.toImage(pixelRatio: 2);
            final bytes =
                await image.toByteData(format: ui.ImageByteFormat.png);
            File('$output/flutter-$plan-${width.toInt()}.png')
                .writeAsBytesSync(bytes!.buffer.asUint8List());
            image.dispose();
          });
        }
        await tester.ensureVisible(find.text('消耗分析').first);
        await tester.pumpAndSettle();
        await tester.tap(find.text('消耗分析').first);
        await tester.pumpAndSettle();
        expect(find.text('5 小时窗口 · Plus'),
            plan == 'plus' ? findsOneWidget : findsNothing);
        if (plan == 'plus') expect(find.text('72% 剩余'), findsOneWidget);
        if (output.isNotEmpty) {
          final render = boundary.currentContext!.findRenderObject()!
              as RenderRepaintBoundary;
          await tester.runAsync(() async {
            final image = await render.toImage(pixelRatio: 2);
            final bytes =
                await image.toByteData(format: ui.ImageByteFormat.png);
            File('$output/flutter-consumption-$plan-${width.toInt()}.png')
                .writeAsBytesSync(bytes!.buffer.asUint8List());
            image.dispose();
          });
        }

        expect(tester.takeException(), isNull);
        await tester.ensureVisible(find.text('使用计划'));
        await tester.pumpAndSettle();
        await tester.tap(find.text('使用计划'));
        await tester.pumpAndSettle();
        expect(tester.takeException(), isNull);
        await tester.ensureVisible(find.text('提醒中心'));
        await tester.pumpAndSettle();
        await tester.tap(find.text('提醒中心'));
        await tester.pumpAndSettle();
        expect(tester.takeException(), isNull);
        await tester.tap(find.text('设备'));
        await tester.pumpAndSettle();
        expect(destination, 3);
        expect(tester.takeException(), isNull);
        await tester.pumpWidget(const SizedBox.shrink());
        await tester.runAsync(store.close);
        api.client.close();
      });
    }
  }
}
