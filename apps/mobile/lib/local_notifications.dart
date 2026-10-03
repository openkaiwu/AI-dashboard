import 'dart:async';
import 'dart:io';

import 'package:flutter_local_notifications/flutter_local_notifications.dart';

/// Heads-up popup for strong reminders. A new channel id is required because
/// Android keeps the importance of an existing channel. iOS mirrors the same
/// three actions through a Darwin notification category so action ids stay
/// identical across platforms (read, snooze, dismiss).
class LocalNotifications {
  static final _plugin = FlutterLocalNotificationsPlugin();
  static bool _ready = false;
  static const channelId = 'aihub_popup_reminders';
  static const categoryId = 'aihub_reminder';

  /// id is the server notification id. action is read, snooze, dismiss, or open.
  static Future<void> Function(String id, String action)? onAction;

  static Future<void> init() async {
    if (_ready) return;
    try {
      const android = AndroidInitializationSettings('@mipmap/ic_launcher');
      // DarwinNotificationAction.plain is a factory constructor, so the
      // category tree cannot be const.
      final ios = DarwinInitializationSettings(
        notificationCategories: [
          DarwinNotificationCategory(
            categoryId,
            actions: [
              DarwinNotificationAction.plain('read', '已读'),
              DarwinNotificationAction.plain('snooze', '1 小时后提醒'),
              DarwinNotificationAction.plain('dismiss', '忽略本次'),
            ],
          ),
        ],
      );
      await _plugin.initialize(
        settings: InitializationSettings(android: android, iOS: ios),
        onDidReceiveNotificationResponse: (response) {
          final id = response.payload;
          if (id == null || id.isEmpty) return;
          final action = (response.actionId == null || response.actionId!.isEmpty)
              ? 'open'
              : response.actionId!;
          unawaited(onAction?.call(id, action));
        },
      );
      final impl = _plugin.resolvePlatformSpecificImplementation<
          AndroidFlutterLocalNotificationsPlugin>();
      await impl?.createNotificationChannel(const AndroidNotificationChannel(
        channelId,
        '弹窗提醒',
        description: '额度与重置提醒会在屏幕上弹出',
        importance: Importance.max,
        playSound: true,
        enableVibration: true,
      ));
      await impl?.requestNotificationsPermission();
      if (Platform.isIOS) {
        await _plugin
            .resolvePlatformSpecificImplementation<
                IOSFlutterLocalNotificationsPlugin>()
            ?.requestPermissions(alert: true, sound: true, badge: true);
      }
      _ready = true;
    } catch (_) {
      _ready = false;
    }
  }

  static Future<void> showReminder({
    required String id,
    required String title,
    required String body,
  }) async {
    if (!_ready) await init();
    if (!_ready) return;
    try {
      final details = NotificationDetails(
        android: AndroidNotificationDetails(
          channelId,
          '弹窗提醒',
          channelDescription: '额度与重置提醒会在屏幕上弹出',
          importance: Importance.max,
          priority: Priority.max,
          category: AndroidNotificationCategory.reminder,
          visibility: NotificationVisibility.public,
          ticker: title,
          styleInformation: BigTextStyleInformation(body),
          playSound: true,
          enableVibration: true,
          fullScreenIntent: true,
          actions: const [
            AndroidNotificationAction('read', '已读', showsUserInterface: true),
            AndroidNotificationAction('snooze', '1 小时后提醒', showsUserInterface: true),
            AndroidNotificationAction('dismiss', '忽略本次', showsUserInterface: true),
          ],
        ),
        iOS: const DarwinNotificationDetails(
          presentAlert: true,
          presentBanner: true,
          presentList: true,
          presentSound: true,
          categoryIdentifier: categoryId,
          interruptionLevel: InterruptionLevel.timeSensitive,
        ),
      );
      final code = id.hashCode & 0x7fffffff;
      await _plugin.show(
        id: code,
        title: title,
        body: body,
        notificationDetails: details,
        payload: id,
      );
    } catch (_) {}
  }
}
