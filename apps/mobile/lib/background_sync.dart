import 'dart:io';
import 'dart:convert';
import 'package:flutter/services.dart';
import 'api.dart';
import 'store.dart';

class BackgroundSync {
  static const channel = MethodChannel('dev.aihub/background');
  static String scope(HubApi api) =>
      '${api.base}|${api.session?['user']?['id']}';
  static Future<void> check(HubApi api) async {
    if (Platform.isAndroid) {
      await renew(api);
      await channel.invokeMethod('check');
    }
  }

  static Future<void> primary(HubApi api) async {
    await api.call('PUT', '/api/v1/mobile/reminders');
    await check(api);
  }

  static Future<Json> status() async => Platform.isAndroid
      ? Map<String, dynamic>.from(await channel.invokeMethod('status') as Map)
      : {};
  static Future<void> bind(HubApi api) async {
    if (Platform.isAndroid) {
      await channel.invokeMethod('bind', {'scope': scope(api)});
    }
  }

  static Future<void> disable() async {
    if (Platform.isAndroid) await channel.invokeMethod('disable');
  }

  static Future<Json> enable(HubApi api) async {
    final token = await api.call('POST', '/api/v1/mobile/background-token');
    return Map<String, dynamic>.from(await channel.invokeMethod(
            'configure', {'server': api.base, 'scope': scope(api), ...token})
        as Map);
  }

  static Future<void> renew(HubApi api) async {
    if (!Platform.isAndroid) return;
    await bind(api);
    final info = await status();
    final expiry = DateTime.tryParse(info['expires_at']?.toString() ?? '');
    if (info['enabled'] == true &&
        (expiry == null ||
            expiry.difference(DateTime.now()).inDays < 2 ||
            info['error']?.toString().contains('授权') == true)) {
      await enable(api);
    }
  }

  static Future<bool> seen(HubApi api, String id, String token) async =>
      Platform.isAndroid &&
      await channel.invokeMethod(
              'seen', {'scope': scope(api), 'id': id, 'token': token}) ==
          true;
  static Future<void> mark(HubApi api, String id, String token) async {
    if (Platform.isAndroid) {
      await channel.invokeMethod(
          'mark', {'scope': scope(api), 'id': id, 'token': token});
    }
  }

  static Future<Json?> cached(HubApi api, String kind) async {
    if (!Platform.isAndroid) return null;
    try {
      final raw = await channel
          .invokeMethod<String>('cache', {'scope': scope(api), 'kind': kind});
      return raw == null ? null : jsonDecode(raw) as Json;
    } catch (_) {
      return null;
    }
  }
}
