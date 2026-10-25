import 'dart:async';
import 'dart:convert';
import 'package:aihub_mobile/api.dart';
import 'package:aihub_mobile/codex_page.dart';
import 'package:aihub_mobile/display_time.dart';
import 'package:aihub_mobile/store.dart';
import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

class MemoryCache extends HubStore {
  MemoryCache({this.failWrites = false}) : super(NativeDatabase.memory());
  final bool failWrites;
  final saved = <String, List<dynamic>>{};
  @override
  Future<List<dynamic>> readCodex(String scope) async => saved[scope] ?? [];
  @override
  Future<void> saveCodex(String scope, List<dynamic> data) async {
    if (failWrites) throw StateError('storage full');
    saved[scope] = data;
  }
}

Json overview(List<String> ids) => {
      'plan': {'plan_type': 'pro'},
      'alerts': [],
      'quiet': false,
      'preferences': {'enabled': true},
      'devices': [
        for (final id in ids)
          {
            'id': id,
            'name': id,
            'snapshot': {
              'status': 'ok',
              'observed_at': DateTime.now().toUtc().toIso8601String()
            },
            'analysis': {
              'state': 'balanced',
              'advice': [],
              'metrics': [
                {
                  'bucket': 'codex',
                  'slot': 'primary',
                  'duration_minutes': 10080,
                  'remaining': 86.0,
                  'hours_left': 124.0,
                  'daily_budget': 16.6,
                  'rate_per_hour': null
                }
              ]
            }
          }
      ]
    };
http.Response reply(Json data) => http.Response(jsonEncode(data), 200,
    headers: {'content-type': 'application/json; charset=utf-8'});
Widget page(HubApi api, HubStore store, {String scope = 'test'}) => MaterialApp(
    home: Scaffold(body: CodexPage(api: api, store: store, scope: scope)));

void main() {
  test('invalid observation time stays unknown', () {
    expect(formatLocalTimestamp(null, empty: '采集时间未知'), '采集时间未知');
    expect(formatLocalTimestamp('not-a-date', empty: '采集时间未知'), '采集时间未知');
  });
  testWidgets('fresh quota survives cache failure and stays usable',
      (tester) async {
    final store = MemoryCache(failWrites: true);
    final api = HubApi('http://localhost', (_) async {},
        client: MockClient((_) async => reply(overview(['电脑 A']))));
    addTearDown(store.close);
    addTearDown(api.client.close);
    await tester.pumpWidget(page(api, store));
    await tester.pumpAndSettle();
    expect(find.text('节奏平稳'), findsOneWidget);
    expect(find.textContaining('最新数据已显示'), findsOneWidget);
    expect(find.text('等待电脑新数据'), findsNothing);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('removed selected device falls back to a current device',
      (tester) async {
    var ids = ['电脑 A', '电脑 B'];
    final store = MemoryCache();
    final api = HubApi('http://localhost', (_) async {},
        client: MockClient((_) async => reply(overview(ids))));
    addTearDown(store.close);
    addTearDown(api.client.close);
    await tester.pumpWidget(page(api, store));
    await tester.pumpAndSettle();
    ids = ['电脑 C', '电脑 D'];
    await tester.tap(find.text('刷新'));
    await tester.pumpAndSettle();
    expect(
        tester
            .widget<DropdownButton<String>>(find.byType(DropdownButton<String>))
            .value,
        '电脑 C');
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('late response cannot overwrite a switched account',
      (tester) async {
    final delayed = Completer<http.Response>();
    final store = MemoryCache();
    final oldApi = HubApi('https://old.localhost', (_) async {},
        client: MockClient((_) => delayed.future));
    final newApi = HubApi('https://new.localhost', (_) async {},
        client: MockClient((_) async => reply(overview(['新账号电脑']))));
    addTearDown(store.close);
    addTearDown(oldApi.client.close);
    addTearDown(newApi.client.close);
    await tester.pumpWidget(page(oldApi, store, scope: 'old'));
    await tester.pump();
    await tester.pumpWidget(page(newApi, store, scope: 'new'));
    await tester.pumpAndSettle();
    delayed.complete(reply(overview(['旧账号电脑'])));
    await tester.pumpAndSettle();
    expect(find.text('新账号电脑 · 已同步'), findsOneWidget);
    expect(find.textContaining('旧账号电脑'), findsNothing);
    expect(store.saved.containsKey('old:advisor'), isFalse);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('overview polling pauses in background and refreshes on resume',
      (tester) async {
    var requests = 0;
    final store = MemoryCache();
    final api =
        HubApi('http://localhost', (_) async {}, client: MockClient((_) async {
      requests++;
      return reply(overview(['电脑']));
    }));
    addTearDown(store.close);
    addTearDown(api.client.close);
    await tester.pumpWidget(page(api, store));
    await tester.pumpAndSettle();
    final initial = requests;
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    await tester.pump(const Duration(minutes: 2));
    expect(requests, initial);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpAndSettle();
    expect(requests, initial + 1);
    await tester.pumpWidget(const SizedBox());
  });
}
