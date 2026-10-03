import 'package:flutter/material.dart';

import 'store.dart';

class StrongReminderCopy {
  static String providerLabel(Json n) {
    final name = (n['provider_name'] as String?)?.trim();
    if (name != null && name.isNotEmpty) return name;
    final slug = (n['provider_slug'] as String?)?.trim();
    if (slug == 'codex') return 'Codex';
    if (slug == 'cursor') return 'Cursor';
    return 'AI Hub';
  }

  static String headline(Json n) =>
      (n['title'] as String?)?.trim().isNotEmpty == true
          ? (n['title'] as String).trim()
          : '额度提醒';

  static String bodyText(Json n) {
    final body = (n['body'] as String?)?.trim() ?? '';
    return body.isEmpty ? '请查看额度与重置时间。' : body;
  }

  static String severityLabel(String? severity) {
    switch (severity) {
      case 'critical':
        return '紧急';
      case 'warning':
        return '强提醒';
      default:
        return '提醒';
    }
  }

  static String dialogTitle(Json n) => '${providerLabel(n)} · ${headline(n)}';

  static String notificationTitle(Json n) {
    final badge = severityLabel(n['severity'] as String?);
    return '[$badge] ${providerLabel(n)} · ${headline(n)}';
  }

  static String notificationBody(Json n) => bodyText(n);

  static String? formatWhen(dynamic value) {
    if (value == null) return null;
    try {
      return DateTime.parse(value as String)
          .toLocal()
          .toString()
          .split('.')
          .first;
    } catch (_) {
      return null;
    }
  }
}

class StrongReminderDialog extends StatelessWidget {
  final Json notification;
  const StrongReminderDialog({super.key, required this.notification});

  @override
  Widget build(BuildContext context) {
    final severity = notification['severity'] as String?;
    final when = StrongReminderCopy.formatWhen(notification['created_at']);
    final lines = StrongReminderCopy.bodyText(notification).split('\n');

    return PopScope(
      canPop: false,
      child: AlertDialog(
        title: Text(StrongReminderCopy.dialogTitle(notification)),
        content: SingleChildScrollView(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              if (severity == 'warning' || severity == 'critical')
                Padding(
                  padding: const EdgeInsets.only(bottom: 12),
                  child: Chip(
                    label: Text(StrongReminderCopy.severityLabel(severity)),
                    backgroundColor: severity == 'critical'
                        ? const Color(0xFFFFE4E4)
                        : const Color(0xFFFFF3D6),
                  ),
                ),
              for (final line in lines)
                if (line.trim().isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 6),
                    child: Text(
                      line.trim(),
                      style: line.startsWith('---')
                          ? Theme.of(context)
                              .textTheme
                              .labelSmall
                              ?.copyWith(color: Colors.black45)
                          : Theme.of(context).textTheme.bodyMedium,
                    ),
                  ),
              if (when != null) ...[
                const SizedBox(height: 8),
                Text(
                  '触发时间：$when',
                  style: Theme.of(context).textTheme.labelSmall,
                ),
              ],
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, 'dismiss'),
            child: const Text('忽略本次'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, 'snooze'),
            child: const Text('1 小时后提醒'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, 'read'),
            child: const Text('已读'),
          ),
        ],
      ),
    );
  }
}
