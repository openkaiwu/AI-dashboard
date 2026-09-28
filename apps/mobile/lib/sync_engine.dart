import 'package:uuid/uuid.dart';
import 'store.dart';

class ApiFailure implements Exception {
  final String code, message;
  final int status;
  ApiFailure(this.status, this.code, this.message);
  @override
  String toString() => message;
}

abstract class Transport {
  Future<Json> call(String method, String path, [Json? body]);
}

class SyncEngine {
  final HubStore store;
  final String scope;
  final Transport transport;
  Future<void>? _syncing;
  SyncEngine(this.store, this.scope, this.transport);
  Future<LocalState> state() => store.readState(scope);
  Future<void> save(String title, String body,
      {String? id, bool delete = false}) async {
    if (!delete &&
        (title.trim().isEmpty || title.length > 65 || body.length > 4000)) {
      throw ArgumentError('标题或内容长度不正确');
    }
    final entity = id ?? const Uuid().v4();
    await store.mutate(scope, (s) {
      if (s.pending.any((p) => p['entity_id'] == entity) ||
          s.conflicts.any((c) => c['operation']['entity_id'] == entity)) {
        throw StateError('请先同步或处理这条便笺的冲突');
      }
      s.pending.add({
        'protocol': 1,
        'operation_id': const Uuid().v4(),
        'entity': 'note',
        'entity_id': entity,
        'base_version': s.notes[entity]?['version'] ?? 0,
        'op': delete ? 'delete' : 'put',
        'payload': {'title': title, 'body': body}
      });
    });
  }

  Future<void> resolve(String operationId, bool keepLocal) =>
      store.mutate(scope, (s) {
        final c = s.conflicts
            .firstWhere((c) => c['operation']['operation_id'] == operationId);
        final op = Map<String, dynamic>.from(c['operation']);
        if (keepLocal) {
          op['operation_id'] = const Uuid().v4();
          op['base_version'] =
              s.notes[op['entity_id']]?['version'] ?? c['current']['version'];
          s.pending.add(op);
        }
        s.conflicts
            .removeWhere((c) => c['operation']['operation_id'] == operationId);
      });
  Future<void> rebuild() async {
    await store.mutate(scope, (s) {
      s.cursor = '';
      s.notes = {};
    });
    await sync();
  }

  Future<void> sync() =>
      _syncing ??= _run().whenComplete(() => _syncing = null);
  Future<void> _run() async {
    final initial = await state();
    for (final op in initial.pending) {
      final result = await transport.call('POST', '/api/v1/sync/push', op);
      await store.mutate(scope, (s) {
        s.pending.removeWhere((p) => p['operation_id'] == op['operation_id']);
        if (result['status'] == 'conflict') {
          s.conflicts.add({'operation': op, 'current': result['current']});
          final current = Map<String, dynamic>.from(result['current']);
          if ((current['version'] as num) > 0) {
            s.notes[op['entity_id']] = current;
          }
        } else {
          final event = Map<String, dynamic>.from(result['event']);
          if ((event['version'] as num) >=
              (s.notes[op['entity_id']]?['version'] ?? 0)) {
            s.notes[op['entity_id']] = event;
          }
        }
      });
    }
    var reset = false;
    while (true) {
      final s = await state();
      try {
        final page = await transport.call('GET',
            '/api/v1/sync/pull?cursor=${Uri.encodeQueryComponent(s.cursor)}');
        await store.mutate(scope, (s) {
          for (final raw in page['events']) {
            final e = Map<String, dynamic>.from(raw);
            if ((e['version'] as num) >=
                (s.notes[e['entity_id']]?['version'] ?? 0)) {
              s.notes[e['entity_id']] = e;
            }
          }
          s.cursor = page['cursor'];
          if (page['has_more'] != true) {
            s.lastSync = DateTime.now().toUtc().toIso8601String();
          }
        });
        if (page['has_more'] != true) break;
      } on ApiFailure catch (e) {
        if (e.code == 'cursor_reset' && !reset) {
          reset = true;
          await store.mutate(scope, (s) {
            s.cursor = '';
            s.notes = {};
          });
          continue;
        }
        rethrow;
      }
    }
  }
}
