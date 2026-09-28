import 'dart:convert';
import 'package:http/http.dart' as http;
import 'store.dart';
import 'sync_engine.dart';

String validateServer(String input) {
  final u = Uri.parse(input.trim());
  if (!u.hasAuthority ||
      u.userInfo.isNotEmpty ||
      u.hasQuery ||
      u.hasFragment ||
      (u.path.isNotEmpty && u.path != '/')) {
    throw ArgumentError('请输入服务器根地址');
  }
  if (u.scheme != 'https' &&
      !(u.scheme == 'http' &&
          ['localhost', '127.0.0.1', '::1'].contains(u.host))) {
    throw ArgumentError('跨网络同步需要 HTTPS；HTTP 仅限本机开发');
  }
  return u.origin;
}

class HubApi implements Transport {
  final String base;
  final http.Client client;
  Json? session;
  final Future<void> Function(Json) persist;
  Future<void>? _refreshing;
  HubApi(String base, this.persist, {this.session, http.Client? client})
      : base = validateServer(base),
        client = client ?? http.Client();
  Future<Json> raw(String method, String path,
      [Json? body, String? token]) async {
    final request = http.Request(method, Uri.parse(base + path))
      ..followRedirects = false;
    request.headers['Content-Type'] = 'application/json';
    if (token != null) {
      request.headers['Authorization'] = 'Bearer $token';
    }
    if (body != null) {
      request.body = jsonEncode(body);
    }
    final response = await http.Response.fromStream(
            await client.send(request).timeout(const Duration(seconds: 15)))
        .timeout(const Duration(seconds: 15));
    Json out;
    try {
      out = jsonDecode(response.body) as Json;
    } catch (_) {
      throw ApiFailure(response.statusCode, 'invalid_response', '服务器响应无效');
    }
    if (response.statusCode >= 300) {
      throw ApiFailure(response.statusCode, out['error'] ?? 'error',
          out['message'] ?? '请求失败');
    }
    return out;
  }

  @override
  Future<Json> call(String method, String path, [Json? body]) async {
    if (session?['blocked'] == true) {
      throw ApiFailure(401, 'session_revoked', '请重新登录；本地修改仍保留');
    }
    try {
      return await raw(method, path, body, session?['token']);
    } on ApiFailure catch (e) {
      if (e.status != 401 ||
          session == null ||
          path.startsWith('/api/v1/auth/')) {
        rethrow;
      }
      await (_refreshing ??= _refresh().whenComplete(() => _refreshing = null));
      return raw(method, path, body, session?['token']);
    }
  }

  Future<void> _refresh() async {
    try {
      final next = await raw('POST', '/api/v1/auth/refresh',
          {'refresh_token': session!['refresh_token']});
      await persist(next);
      session = next;
    } on ApiFailure catch (e) {
      if (e.status == 401) {
        session = {...session!, 'blocked': true};
        await persist(session!);
      }
      rethrow;
    }
  }

  Future<void> signIn(
      String email, String password, String name, String installationId,
      {String kind = 'mobile'}) async {
    final result = await raw(
        'POST',
        '/api/v1/auth/login',
        {'email': email, 'password': password, 'device_name': name,
         'device_kind': kind, 'installation_id': installationId});
    await persist(result);
    session = result;
  }
}
