import 'dart:async';
import 'dart:convert';
import 'package:aihub_mobile/api.dart';
import 'package:aihub_mobile/consumption_panel.dart';
import 'package:aihub_mobile/theme.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  test('missing data stays unknown, zero is a real observation', () {
    final now = DateTime.utc(2026, 10, 11);
    expect(consumptionTotal(consumptionPoints(null, 7, now: now)), isNull);
    final data = {
      'series': [
        {'bucket_start': '2026-10-11', 'consumed_pp': 0, 'coverage_hours': 1},
        {'bucket_start': '2026-09-01', 'consumed_pp': 90, 'coverage_hours': 24}
      ]
    };
    expect(consumptionTotal(consumptionPoints(data, 7, now: now)), 0);
  });
  test('daily ranges follow account timezone and preserve gaps', () {
    final points = consumptionPoints({
      'tz_offset_minutes': 480,
      'series': [
        {'bucket_start': '2026-10-11', 'consumed_pp': 3, 'coverage_hours': 1}
      ]
    }, 7, now: DateTime.utc(2026, 10, 10, 17));
    expect(points.last.label, '10/11');
    expect(points.length, 7);
    expect(consumptionTotal(points), 3);
    expect(points.where((p) => p.value != null).length, 1);
  });

  Future<void> mount(WidgetTester tester, HubApi api) async {
    tester.view.physicalSize = const Size(390, 1000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(MaterialApp(
        theme: HubTheme.light(),
        home: Scaffold(
            body: SingleChildScrollView(
                child: Padding(
                    padding: const EdgeInsets.all(18),
                    child: ConsumptionPanel(
                        api: api, plan: 'pro', onConnections: () {}))))));
    await tester.pumpAndSettle();
  }

  http.Response json(Object data) => http.Response(jsonEncode(data), 200,
      headers: {'content-type': 'application/json'});
  final empty = {'series': [], 'hourly': [], 'generations': []};
  testWidgets('failure has no fake zero totals and retry recovers',
      (tester) async {
    var failing = true;
    final api = HubApi('http://localhost', (_) async {},
        client: MockClient((request) async {
      expect(request.url.queryParameters['window'], '10080');
      return failing ? http.Response('{}', 503) : json(empty);
    }));
    await mount(tester, api);
    expect(find.text('暂时无法获取消耗数据'), findsOneWidget);
    expect(find.text('0.0'), findsNothing);
    failing = false;
    await tester.tap(find.text('重新加载'));
    await tester.pumpAndSettle();
    expect(find.text('这段时间还没有消耗样本'), findsOneWidget);
    expect(find.text('暂时无法获取消耗数据'), findsNothing);
    expect(tester.takeException(), isNull);
    api.client.close();
  });
  testWidgets('refresh failure preserves successful real-zero samples',
      (tester) async {
    var failing = false;
    final day = DateTime.now().toUtc().toIso8601String().substring(0, 10);
    final api = HubApi('http://localhost', (_) async {},
        client: MockClient((request) async => failing
            ? http.Response('{}', 503)
            : json({
                'series': [
                  {'bucket_start': day, 'consumed_pp': 0, 'coverage_hours': 1}
                ],
                'hourly': [],
                'generated_at': DateTime.now().toIso8601String()
              })));
    await mount(tester, api);
    expect(find.text('0.0'), findsNWidgets(2));
    failing = true;
    await tester.tap(find.text('刷新数据').first);
    await tester.pumpAndSettle();
    expect(find.text('0.0'), findsNWidgets(2));
    expect(find.textContaining('刷新失败，保留上次数据'), findsOneWidget);
    expect(tester.takeException(), isNull);
    api.client.close();
  });
  testWidgets('loading is distinct from empty', (tester) async {
    final response = Completer<http.Response>();
    final api = HubApi('http://localhost', (_) async {},
        client: MockClient((_) => response.future));
    await tester.pumpWidget(MaterialApp(
        theme: HubTheme.light(),
        home: Scaffold(
            body: SingleChildScrollView(
                child: ConsumptionPanel(
                    api: api, plan: 'pro', onConnections: () {})))));
    await tester.pump();
    expect(find.text('正在读取消耗记录…'), findsOneWidget);
    expect(find.text('这段时间还没有消耗样本'), findsNothing);
    response.complete(json(empty));
    await tester.pumpAndSettle();
    expect(find.text('这段时间还没有消耗样本'), findsOneWidget);
    api.client.close();
  });
}
