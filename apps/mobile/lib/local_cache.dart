import 'dart:convert';
import 'dart:io';
import 'package:path_provider/path_provider.dart';
import 'store.dart';

class LocalCache {
  static String keyHash(String key) {
    var hash = 0x811c9dc5;
    for (final byte in utf8.encode(key)) {
      hash = ((hash ^ byte) * 0x01000193) & 0xffffffff;
    }
    return hash.toRadixString(16);
  }

  static Future<File> _file(String key) async {
    final root = await getApplicationSupportDirectory();
    final dir = Directory('${root.path}/local-data');
    await dir.create(recursive: true);
    return File('${dir.path}/${keyHash(key)}.json');
  }

  static Future<void> save(String key, Json value) async {
    final file = await _file(key);
    final temporary =
        File('${file.path}.${DateTime.now().microsecondsSinceEpoch}.tmp');
    await temporary.writeAsString(jsonEncode({'key': key, 'value': value}),
        flush: true);
    await temporary.rename(file.path);
  }

  static Future<Json?> read(String key) async {
    try {
      final record =
          jsonDecode(await (await _file(key)).readAsString()) as Json;
      return record['key'] == key ? record['value'] as Json : null;
    } catch (_) {
      return null;
    }
  }
}
