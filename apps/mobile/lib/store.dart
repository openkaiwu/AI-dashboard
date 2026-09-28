import 'dart:convert';
import 'package:drift/drift.dart';

typedef Json = Map<String, dynamic>;

class LocalState {
  String cursor;
  String? lastSync;
  Map<String, Json> notes;
  List<Json> pending;
  List<Json> conflicts;
  LocalState(
      {this.cursor = '',
      this.lastSync,
      Map<String, Json>? notes,
      List<Json>? pending,
      List<Json>? conflicts})
      : notes = notes ?? {},
        pending = pending ?? [],
        conflicts = conflicts ?? [];
}

/// Explicit SQL keeps the M0 contract visible; all writes and cursor application are transactional.
class HubStore extends GeneratedDatabase {
  HubStore(super.executor);
  @override
  int get schemaVersion => 1;
  @override
  Iterable<TableInfo<Table, Object?>> get allTables => [];
  @override
  List<DatabaseSchemaEntity> get allSchemaEntities => [];
  @override
  MigrationStrategy get migration => MigrationStrategy(onCreate: (_) async {
        await customStatement(
            'CREATE TABLE profiles(id TEXT PRIMARY KEY,name TEXT NOT NULL,url TEXT NOT NULL)');
        await customStatement(
            'CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL)');
        await customStatement(
            'CREATE TABLE sync_state(scope TEXT PRIMARY KEY,cursor TEXT NOT NULL,last_sync TEXT)');
        await customStatement(
            'CREATE TABLE notes(scope TEXT NOT NULL,id TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(scope,id))');
        await customStatement(
            'CREATE TABLE pending_ops(scope TEXT NOT NULL,operation_id TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(scope,operation_id))');
        await customStatement(
            'CREATE TABLE conflicts(scope TEXT NOT NULL,operation_id TEXT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(scope,operation_id))');
      });

  Future<List<Json>> profiles() async =>
      (await customSelect('SELECT * FROM profiles ORDER BY name').get())
          .map((r) => r.data)
          .toList();
  Future<void> saveProfile(Json p) => customStatement(
      'INSERT INTO profiles(id,name,url) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,url=excluded.url',
      [p['id'], p['name'], p['url']]);
  Future<void> deleteProfile(String id) =>
      customStatement('DELETE FROM profiles WHERE id=?', [id]);
  Future<void> setActive(String id) => customStatement(
      "INSERT INTO settings(key,value) VALUES('active',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
      [id]);
  Future<String?> active() async =>
      (await customSelect("SELECT value FROM settings WHERE key='active'")
              .getSingleOrNull())
          ?.read<String>('value');

  Future<void> saveCodex(String scope, List<dynamic> bridges) => customStatement(
      'INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value',
      ['codex:$scope', jsonEncode(bridges)]);
  Future<List<dynamic>> readCodex(String scope) async {
    final row = await customSelect('SELECT value FROM settings WHERE key=?',
        variables: [Variable('codex:$scope')]).getSingleOrNull();
    return row == null
        ? []
        : jsonDecode(row.read<String>('value')) as List<dynamic>;
  }

  Future<LocalState> readState(String scope) async {
    final row = await customSelect('SELECT * FROM sync_state WHERE scope=?',
        variables: [Variable(scope)]).getSingleOrNull();
    Future<List<Json>> rows(String table) async => (await customSelect(
            'SELECT payload FROM $table WHERE scope=? ORDER BY rowid',
            variables: [Variable(scope)]).get())
        .map((r) => jsonDecode(r.read<String>('payload')) as Json)
        .toList();
    final notes = await rows('notes');
    return LocalState(
        cursor: row?.read<String>('cursor') ?? '',
        lastSync: row?.readNullable<String>('last_sync'),
        notes: {for (final n in notes) n['entity_id'] as String: n},
        pending: await rows('pending_ops'),
        conflicts: await rows('conflicts'));
  }

  Future<void> mutate(String scope, void Function(LocalState) change) =>
      transaction(() async {
        final state = await readState(scope);
        change(state);
        await customStatement(
            'INSERT INTO sync_state(scope,cursor,last_sync) VALUES(?,?,?) ON CONFLICT(scope) DO UPDATE SET cursor=excluded.cursor,last_sync=excluded.last_sync',
            [scope, state.cursor, state.lastSync]);
        for (final table in ['notes', 'pending_ops', 'conflicts']) {
          await customStatement('DELETE FROM $table WHERE scope=?', [scope]);
        }
        for (final note in state.notes.values) {
          await customStatement(
              'INSERT INTO notes(scope,id,payload) VALUES(?,?,?)',
              [scope, note['entity_id'], jsonEncode(note)]);
        }
        for (final op in state.pending) {
          await customStatement(
              'INSERT INTO pending_ops(scope,operation_id,payload) VALUES(?,?,?)',
              [scope, op['operation_id'], jsonEncode(op)]);
        }
        for (final c in state.conflicts) {
          await customStatement(
              'INSERT INTO conflicts(scope,operation_id,payload) VALUES(?,?,?)',
              [scope, c['operation']['operation_id'], jsonEncode(c)]);
        }
      });

  Future<void> clearScope(String scope) => transaction(() async {
        for (final table in ['sync_state', 'notes', 'pending_ops', 'conflicts']) {
          await customStatement('DELETE FROM $table WHERE scope=?', [scope]);
        }
        await customStatement('DELETE FROM settings WHERE key=?', ['codex:$scope']);
      });
}
