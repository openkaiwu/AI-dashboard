import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:aihub_mobile/api.dart';
import 'package:aihub_mobile/store.dart';
import 'package:aihub_mobile/sync_engine.dart';

void main() {
  test(
      'revoked refresh persists blocked session and stops repeated network calls',
      () async {
    var count = 0;
    Json? saved;
    final api = HubApi(
        'https://test.invalid',
        (s) async {
          saved = s;
        },
        session: {
          'token': 'old',
          'refresh_token': 'refresh',
          'user': {'id': 'user'}
        },
        client: MockClient((r) async {
          count++;
          return http.Response(
              jsonEncode({'error': 'session_revoked', 'message': 'revoked'}),
              401);
        }));
    addTearDown(api.client.close);
    await expectLater(
        api.call('GET', '/api/v1/sync/pull'), throwsA(isA<ApiFailure>()));
    expect(saved!['blocked'], true);
    await expectLater(
        api.call('GET', '/api/v1/sync/pull'), throwsA(isA<ApiFailure>()));
    expect(count, 2);
    expect(saved!['user']['id'], 'user');
  });
}
