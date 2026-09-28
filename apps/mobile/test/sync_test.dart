import 'dart:io';
import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:aihub_mobile/store.dart';
import 'package:aihub_mobile/sync_engine.dart';

class Fake implements Transport {
  Future<Json> Function(String, String, Json?) handler;
  Fake(this.handler);
  @override
  Future<Json> call(String m, String p, [Json? b]) => handler(m, p, b);
}

Json event(Json op, int version, {String? title}) => {
      'entity_id': op['entity_id'],
      'entity': 'note',
      'op': op['op'],
      'version': version,
      'seq': version,
      'payload':
          title == null ? op['payload'] : {'title': title, 'body': 'remote'}
    };
Json page(List<Json> events, {String cursor = 'epoch:1', bool more = false}) =>
    {'cursor': cursor, 'events': events, 'has_more': more};
void main() {
  test(
      'offline edit, real SQLite close/reopen, lost response retry keeps operation ID',
      () async {
    final dir = await Directory.systemTemp.createTemp('aihub-test-');
    addTearDown(() => dir.delete(recursive: true));
    final file = File('${dir.path}/cache.sqlite');
    var store = HubStore(NativeDatabase(file));
    final unavailable =
        Fake((m, p, b) async => throw const SocketException('offline'));
    var engine = SyncEngine(store, 'server-a|user-1', unavailable);
    await engine.save('Offline title', 'Local body', id: 'note');
    await expectLater(engine.sync(), throwsA(isA<SocketException>()));
    final operation = (await engine.state()).pending.single;
    await store.close();
    store = HubStore(NativeDatabase(file));
    addTearDown(store.close);
    var calls = 0;
    final transport = Fake((m, p, b) async {
      if (m == 'POST') {
        expect(b!['operation_id'], operation['operation_id']);
        calls++;
        if (calls == 1) throw const SocketException('response lost');
        return {'status': 'applied', 'event': event(b, 1)};
      }
      return page([event(operation, 1)]);
    });
    engine = SyncEngine(store, 'server-a|user-1', transport);
    await expectLater(engine.sync(), throwsA(isA<SocketException>()));
    expect((await engine.state()).pending.single['operation_id'],
        operation['operation_id']);
    await engine.sync();
    final state = await engine.state();
    expect(state.pending, isEmpty);
    expect(state.notes['note']!['payload']['title'], 'Offline title');
    expect(state.cursor, 'epoch:1');
    expect((await store.readState('server-b|user-1')).notes, isEmpty);
    expect((await store.readState('server-a|user-2')).notes, isEmpty);
  });
  test(
      'conflict preserves draft; explicit resolution uses new id and latest version',
      () async {
    final store = HubStore(NativeDatabase.memory());
    addTearDown(store.close);
    var conflicted = true;
    final transport = Fake((m, p, b) async {
      if (m == 'POST') {
        return conflicted
            ? {
                'status': 'conflict',
                'current': event(b!, 3, title: 'Server title')
              }
            : {'status': 'applied', 'event': event(b!, 4)};
      }
      return page([]);
    });
    final engine = SyncEngine(store, 'scope', transport);
    await engine.save('My draft', 'local', id: 'note');
    final old = (await engine.state()).pending.single['operation_id'];
    await engine.sync();
    var s = await engine.state();
    expect(s.conflicts.single['operation']['payload']['title'], 'My draft');
    expect(s.notes['note']!['version'], 3);
    await engine.resolve(old, true);
    s = await engine.state();
    expect(s.pending.single['base_version'], 3);
    expect(s.pending.single['operation_id'], isNot(old));
    conflicted = false;
    await engine.sync();
    expect((await engine.state()).notes['note']!['version'], 4);
  });
  test(
      'interrupted pull commits cursor only with its page; reset replays tombstones',
      () async {
    final store = HubStore(NativeDatabase.memory());
    addTearDown(store.close);
    var step = 0;
    final note = {
      'entity_id': 'note',
      'op': 'put',
      'version': 1,
      'payload': {'title': 'remote', 'body': ''}
    };
    final deletion = {
      'entity_id': 'note',
      'op': 'delete',
      'version': 2,
      'payload': {'title': '', 'body': ''}
    };
    final transport = Fake((m, p, b) async {
      step++;
      switch (step) {
        case 1:
          return page([note], cursor: 'epoch:1', more: true);
        case 2:
          throw const SocketException('pull interrupted');
        case 3:
          expect(p, contains('epoch%3A1'));
          throw ApiFailure(409, 'cursor_reset', 'reset');
        case 4:
          expect(p, endsWith('cursor='));
          return page([note, deletion], cursor: 'new:2');
        default:
          throw StateError('unexpected request');
      }
    });
    final engine = SyncEngine(store, 'scope', transport);
    await expectLater(engine.sync(), throwsA(isA<SocketException>()));
    expect((await engine.state()).cursor, 'epoch:1');
    await engine.sync();
    final s = await engine.state();
    expect(s.cursor, 'new:2');
    expect(s.notes['note']!['op'], 'delete');
  });
  test('revocation stops upload, preserves queue, and never starts pull',
      () async {
    final store = HubStore(NativeDatabase.memory());
    addTearDown(store.close);
    var calls = 0;
    final engine = SyncEngine(store, 'scope', Fake((m, p, b) async {
      calls++;
      throw ApiFailure(401, 'session_revoked', 'revoked');
    }));
    await engine.save('Draft', 'body');
    await expectLater(engine.sync(), throwsA(isA<ApiFailure>()));
    expect(calls, 1);
    expect((await engine.state()).pending.length, 1);
  });
  test('profile CRUD and schema survive file restart', () async {
    final dir = await Directory.systemTemp.createTemp('aihub-migration-');
    addTearDown(() => dir.delete(recursive: true));
    final file = File('${dir.path}/store.sqlite');
    var db = HubStore(NativeDatabase(file));
    await db.saveProfile({'id': 'a', 'name': 'A', 'url': 'https://a.example'});
    await db.saveProfile({'id': 'b', 'name': 'B', 'url': 'https://b.example'});
    await db.setActive('b');
    await db.close();
    db = HubStore(NativeDatabase(file));
    addTearDown(db.close);
    expect((await db.profiles()).length, 2);
    expect(await db.active(), 'b');
    await db.saveProfile(
        {'id': 'b', 'name': 'Renamed', 'url': 'https://b.example'});
    await db.deleteProfile('a');
    expect((await db.profiles()).single['name'], 'Renamed');
  });
}
