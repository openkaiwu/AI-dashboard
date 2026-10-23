import 'dart:async';
import 'dart:io';

import 'package:flutter_local_notifications/flutter_local_notifications.dart';

/// Heads-up popup for strong reminders. Preserve the existing channel and the
/// user's notification settings. iOS mirrors the same
/// three actions through a Darwin notification category so action ids stay
/// identical across platforms (read, snooze, dismiss).
class LocalNotifications {
  static final _plugin = FlutterLocalNotificationsPlugin();
  static bool _ready = false;
  static const channelId = 'aihub_popup_reminders_v2';
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
              DarwinNotificationAction.plain('read', '已读',
                  options: {DarwinNotificationActionOption.foreground}),
              DarwinNotificationAction.plain('dismiss', '忽略本次',
                  options: {DarwinNotificationActionOption.foreground}),
            ],
          ),
        ],
      );
      await _plugin.initialize(
        settings: InitializationSettings(android: android, iOS: ios),
        onDidReceiveNotificationResponse: (response) {
          final id = response.payload;
          if (id == null || id.isEmpty) return;
          final action =
              (response.actionId == null || response.actionId!.isEmpty)
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
        description: '额度与到期提醒，横幅展示受系统设置影响',
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

  static Future<void> requestPermission() async {
    await init();
    await _plugin
        .resolvePlatformSpecificImplementation<
            AndroidFlutterLocalNotificationsPlugin>()
        ?.requestNotificationsPermission();
  }

  static int stableId(String id) {
    var hash = 0;
    for (final unit in id.codeUnits) {
      hash = (31 * hash + unit) & 0x7fffffff;
    }
    return hash;
  }

  static Future<bool> showReminder({
    required String id,
    required String title,
    required String body,
  }) async {
    if (!_ready) await init();
    if (!_ready) return false;
    try {
      if (Platform.isAndroid) {
        final android = _plugin.resolvePlatformSpecificImplementation<
            AndroidFlutterLocalNotificationsPlugin>();
        if (await android?.areNotificationsEnabled() != true) return false;
        final channels = await android?.getNotificationChannels();
        if (channels?.any((channel) =>
                channel.id == channelId &&
                channel.importance == Importance.none) ==
            true) {
          return false;
        }
      }
      if (Platform.isIOS) {
        final ios = _plugin.resolvePlatformSpecificImplementation<
            IOSFlutterLocalNotificationsPlugin>();
        final permission = await ios?.checkPermissions();
        if (permission?.isEnabled != true &&
            permission?.isProvisionalEnabled != true) {
          return false;
        }
      }
      final details = NotificationDetails(
        android: AndroidNotificationDetails(
          channelId,
          '弹窗提醒',
          channelDescription: '额度与到期提醒，横幅展示受系统设置影响',
          importance: Importance.max,
          priority: Priority.max,
          category: AndroidNotificationCategory.reminder,
          visibility: NotificationVisibility.public,
          ticker: title,
          styleInformation: BigTextStyleInformation(body),
          playSound: true,
          enableVibration: true,
          fullScreenIntent: false,
          actions: const [
            AndroidNotificationAction('read', '已读', showsUserInterface: true),
            AndroidNotificationAction('dismiss', '忽略本次',
                showsUserInterface: true),
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
      final code = stableId(id);
      await _plugin.show(
        id: code,
        title: title,
        body: body,
        notificationDetails: details,
        payload: id,
      );
      return true;
    } catch (_) {
      return false;
    }
  }

  static Future<void> cancelReminder(String id) async {
    if (_ready) await _plugin.cancel(id: stableId(id));
  }
}
