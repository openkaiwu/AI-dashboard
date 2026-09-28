import 'dart:io';
import 'package:drift/drift.dart' show driftRuntimeOptions;
import 'package:flutter_test/flutter_test.dart';
import 'package:drift/native.dart';
import 'package:uuid/uuid.dart';
import 'package:aihub_mobile/api.dart';
import 'package:aihub_mobile/store.dart';
import 'package:aihub_mobile/sync_engine.dart';

void main() {
  // Two independent executors model two devices; never share one executor.
  driftRuntimeOptions.dontWarnAboutMultipleDatabases = true;
  final base = Platform.environment['AIHUB_INTEGRATION_URL'];
  final integrationEmail = Platform.environment['AIHUB_INTEGRATION_EMAIL'];
  final codexEmail = Platform.environment['AIHUB_INTEGRATION_CODEX_EMAIL'];
  final integrationPassword = Platform.environment['AIHUB_INTEGRATION_PASSWORD'];
  test('two Drift clients converge through real Go/PostgreSQL HTTP API',
      () async {
    final a = HubApi(base!, (_) async {}), b = HubApi(base, (_) async {});
    addTearDown(a.client.close);
    addTearDown(b.client.close);
    await a.signIn(integrationEmail!, integrationPassword!, 'Flutter A', const Uuid().v4(), kind:'desktop');
    await b.signIn(integrationEmail, integrationPassword, 'Flutter B', const Uuid().v4());
    final sa = HubStore(NativeDatabase.memory()),
        sb = HubStore(NativeDatabase.memory());
    addTearDown(sa.close);
    addTearDown(sb.close);
    final ea = SyncEngine(sa, 'a', a), eb = SyncEngine(sb, 'b', b);
    await ea.save('Desktop draft', 'offline A', id: 'shared');
    await eb.save('Phone draft', 'offline B', id: 'shared');
    await ea.sync();
    await eb.sync();
    expect((await eb.state()).conflicts.length, 1);
    await eb.resolve(
        (await eb.state()).conflicts.single['operation']['operation_id'], true);
    await eb.sync();
    await ea.sync();
    expect(
        (await ea.state()).notes['shared']!['payload']['title'], 'Phone draft');
    await ea.save('', '', id: 'shared', delete: true);
    await ea.sync();
    await eb.sync();
    expect((await eb.state()).notes['shared']!['op'], 'delete');
    await expectLater(a.call('DELETE', '/api/v1/devices/${b.session!['device_id']}'),
        throwsA(isA<ApiFailure>()));
  }, skip: base == null || integrationEmail == null || integrationPassword == null ? 'provisioned integration account required' : false);
  test('Codex desktop upload reaches mobile and survives cache reopen',
      () async {
    final desktop = HubApi(base!, (_) async {}),
        mobile = HubApi(base, (_) async {});
    addTearDown(desktop.client.close);
    addTearDown(mobile.client.close);
    await desktop.signIn(codexEmail!, integrationPassword!, 'Desktop', const Uuid().v4(), kind:'desktop');
    await mobile.signIn(codexEmail, integrationPassword, 'Phone', const Uuid().v4());
    final connection = await desktop
        .call('POST', '/api/v1/codex/bridges', {'name': 'Desktop Codex'});
    await desktop.raw(
        'POST',
        '/api/v1/codex/snapshot',
        {
          'observed_at': DateTime.now().toUtc().toIso8601String(),
          'source': 'codex_app_server',
          'status': 'ok',
          'buckets': [
            {
              'id': 'codex',
              'primary': {
                'used_percent': 30,
                'duration_minutes': 10080,
                'resets_at': 1790475828
              },
              'secondary': null
            }
          ]
        },
        connection['token']);
    final payload = await mobile.call('GET', '/api/v1/codex/bridges');
    final analysis = await mobile.call('GET', '/api/v1/codex/overview');
    expect(
        analysis['devices'].single['analysis']['metrics'].single['remaining'],
        70);
    expect(analysis['preferences']['enabled'], true);

    final dir = await Directory.systemTemp.createTemp('codex-mobile-');
    addTearDown(() => dir.delete(recursive: true));
    final file = File('${dir.path}/cache.sqlite');
    var cache = HubStore(NativeDatabase(file));
    await cache.saveCodex('server|user', payload['bridges']);
    await cache.saveCodex('server|user:advisor', [analysis]);
    await cache.close();
    cache = HubStore(NativeDatabase(file));
    addTearDown(cache.close);
    final rows = await cache.readCodex('server|user');
    expect(rows.single['snapshot']['buckets'].single['primary']['used_percent'],
        30);
    expect(await cache.readCodex('other-server|user'), isEmpty);
    expect(
        (await cache.readCodex('server|user:advisor')).single['preferences']
            ['enabled'],
        true);
    await mobile.call('DELETE', '/api/v1/codex/bridges/${connection['id']}');
    expect(
        (await mobile.call('GET', '/api/v1/codex/bridges'))['bridges']
            .single['revoked_at'],
        isNotNull);
  }, skip: base == null || codexEmail == null || integrationPassword == null ? 'provisioned Codex integration account required' : false);
}
