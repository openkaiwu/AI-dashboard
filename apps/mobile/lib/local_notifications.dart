import 'package:flutter_local_notifications/flutter_local_notifications.dart';

/// System-level local notifications for strong reminders (INH-353 v1):
/// warning/critical server notifications surface in the Android notification
/// tray while the app is running. Background push (FCM) stays out of scope.
class LocalNotifications {
  static final _plugin = FlutterLocalNotificationsPlugin();
  static bool _ready = false;

  static Future<void> init() async {
    if (_ready) return;
    try {
      const android = AndroidInitializationSettings('@mipmap/ic_launcher');
      await _plugin.initialize(
        settings: const InitializationSettings(android: android),
      );
      final impl = _plugin.resolvePlatformSpecificImplementation<
          AndroidFlutterLocalNotificationsPlugin>();
      await impl?.requestNotificationsPermission();
      _ready = true;
    } catch (_) {
      _ready = false; // notifications are best-effort; never block startup
    }
  }

  static Future<void> showReminder({required String id, required String title, required String body}) async {
    if (!_ready) return;
    try {
      const details = NotificationDetails(
        android: AndroidNotificationDetails(
          'aihub_reminders',
          'AI Hub 提醒',
          channelDescription: '额度与重置提醒',
          importance: Importance.high,
          priority: Priority.high,
          styleInformation: BigTextStyleInformation(''),
        ),
      );
      // Notification ids are 32-bit; derive a stable one from the server id.
      final code = id.hashCode & 0x7fffffff;
      await _plugin.show(
        id: code,
        title: title,
        body: body,
        notificationDetails: details,
        payload: id,
      );
    } catch (_) {
      // best-effort
    }
  }
}
