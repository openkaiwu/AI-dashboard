import 'dart:convert';
import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:aihub_mobile/api.dart';
import 'package:aihub_mobile/codex_page.dart';
import 'package:aihub_mobile/cursor_page.dart';
import 'package:aihub_mobile/manual_account.dart';
import 'package:aihub_mobile/store.dart';

void main() {
  testWidgets(
      'Android Codex radar marks old checks stale and shows source link',
      (tester) async {
    tester.view.physicalSize = const Size(430, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final store = HubStore(NativeDatabase.memory());
    addTearDown(store.close);
    final old = DateTime.now()
        .toUtc()
        .subtract(const Duration(hours: 4))
        .toIso8601String();
    final overview = {
      'devices': [
        {
          'id': 'bridge-1',
          'name': '电脑',
          'snapshot': {
            'observed_at': DateTime.now().toUtc().toIso8601String(),
            'status': 'ok',
            'news': {
              'checked_at': old,
              'status': 'checked',
              'items': [
                {
                  'published_at': old,
                  'summary': '已核实的旧记录',
                  'url': 'https://x.com/thsottiaux/status/123'
                }
              ]
            }
          },
          'history': [],
          'analysis': {'state': 'balanced', 'metrics': [], 'advice': []}
        }
      ],
      'preferences': {
        'enabled': true,
        'near_hours': 24,
        'spare_percent': 35,
        'low_percent': 15,
        'pace_lead': 20,
        'credit_hours': 24,
        'cooldown_hours': 6,
        'quiet_start': 23,
        'quiet_end': 8,
        'utc_offset_minutes': 480
      },
      'plan': {'plan_type': 'pro'},
      'alerts': [],
      'quiet': false,
    };
    Map<String, dynamic>? savedPreferences;
    final api = HubApi('https://hub.example.com', (_) async {},
        session: {'token': 'test'}, client: MockClient((request) async {
      if (request.method == 'PATCH' &&
          request.url.path == '/api/v1/codex/preferences') {
        savedPreferences = jsonDecode(request.body) as Map<String, dynamic>;
      }
      return http.Response(jsonEncode(overview), 200,
          headers: {'content-type': 'application/json'});
    }));
    addTearDown(api.client.close);
    await tester.pumpWidget(MaterialApp(
        home: Scaffold(
            body:
                CodexPage(api: api, store: store, scope: 'test|user|phone'))));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.text('重置雷达').first);
    await tester.tap(find.text('重置雷达').first);
    await tester.pumpAndSettle();
    expect(find.text('检查记录已过期'), findsOneWidget);
    expect(find.text('查看原帖 ↗'), findsOneWidget);
    await tester.ensureVisible(find.text('提醒设置').first);
    await tester.tap(find.text('提醒设置').first);
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.text('临近重置 · 小时'));
    await tester.tap(find.text('临近重置 · 小时'));
    await tester.pumpAndSettle();
    await tester.enterText(find.byType(TextField).last, '12');
    await tester.tap(find.text('保存'));
    await tester.pumpAndSettle();
    expect(savedPreferences?['near_hours'], 12);
  });

  testWidgets('Android Cursor rules can preview server evaluation',
      (tester) async {
    final api = HubApi('https://hub.example.com', (_) async {},
        session: {'token': 'test'}, client: MockClient((request) async {
      Object data = {};
      switch (request.url.path) {
        case '/api/v1/dashboard':
          data = {
            'accounts': [
              {
                'id': 'acct-1',
                'display_name': 'Cursor 本机',
                'provider': {'slug': 'cursor'},
                'buckets': []
              }
            ]
          };
        case '/api/v1/notifications':
          data = {'notifications': []};
        case '/api/v1/notification-rules':
          data = {
            'rules': [
              {
                'id': 'rule-1',
                'name': '低额度',
                'rule_type': 'quota.low',
                'enabled': true,
                'params': {'ratio': 0.2},
                'provider_account_id': 'acct-1'
              }
            ]
          };
        case '/api/v1/notification-rules/rule-1/preview':
          data = {
            'would_fire': true,
            'matches': [
              {
                'provider': 'Cursor',
                'account': 'Cursor 本机',
                'would_fire': true,
                'reason': '低于阈值'
              }
            ]
          };
      }
      return http.Response(jsonEncode(data), 200,
          headers: {'content-type': 'application/json'});
    }));
    addTearDown(api.client.close);
    await tester.pumpWidget(MaterialApp(
        home: Scaffold(
            body: SizedBox(height: 850, child: CursorPage(api: api)))));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.text('提醒设置').first);
    await tester.tap(find.text('提醒设置').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('按当前数据预览'));
    await tester.pumpAndSettle();
    expect(find.text('将触发'), findsOneWidget);
    expect(find.textContaining('低于阈值'), findsOneWidget);
  });

  testWidgets('manual Cursor quota is sent with user_manual server contract',
      (tester) async {
    Map<String, dynamic>? created;
    final api = HubApi('https://hub.example.com', (_) async {},
        session: {'token': 'test'}, client: MockClient((request) async {
      if (request.url.path == '/api/v1/providers') {
        return http.Response(
            jsonEncode({
              'providers': [
                {'id': 'provider-cursor', 'slug': 'cursor'}
              ]
            }),
            200);
      }
      if (request.url.path == '/api/v1/provider-accounts') {
        created = jsonDecode(request.body) as Map<String, dynamic>;
        return http.Response('{"id":"acct-new"}', 201);
      }
      return http.Response('{}', 404);
    }));
    addTearDown(api.client.close);
    await tester.pumpWidget(MaterialApp(
        home: Scaffold(
            body: ManualAccountSheet(
                api: api, providerSlug: 'cursor', onSaved: () {}))));
    await tester.enterText(find.byType(TextField).at(0), '我的 Cursor');
    await tester.enterText(find.byType(TextField).at(3), '100');
    await tester.enterText(find.byType(TextField).at(4), '25');
    await tester.ensureVisible(find.text('保存到当前账户'));
    await tester.tap(find.text('保存到当前账户'));
    await tester.pumpAndSettle();
    expect(created?['provider_id'], 'provider-cursor');
    expect(created?['display_name'], '我的 Cursor');
    expect((created?['quota'] as Map)['remaining_value'], 25);
  });
}
