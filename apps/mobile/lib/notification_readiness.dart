({bool ready, String label, String? hint}) notificationReadiness(
    Map<String, dynamic> info) {
  if (info['notifications'] != true) {
    return (ready: false, label: '通知未允许', hint: '请允许系统通知；关闭通知权限时，应用内提醒也不会弹出。');
  }
  switch (info['notification_state']) {
    case 'channel_blocked':
      return (
        ready: false,
        label: '额度通知已关闭',
        hint: '请在“通知与横幅设置”中开启额度通知渠道。渠道关闭时，应用内提醒也不会弹出。'
      );
    case 'channel_missing':
      return (ready: false, label: '通知渠道待检查', hint: '请打开“通知与横幅设置”检查额度通知渠道。');
    case 'system_dnd':
      return (ready: false, label: '系统免打扰中', hint: '系统免打扰开启期间，应用内弹窗也会保持安静。');
    case 'tray_only':
      return (
        ready: true,
        label: '通知栏已允许',
        hint: '系统仅允许通知栏通知，可在“通知与横幅设置”中调整。应用内提醒仍按规则检查。'
      );
    default:
      return (ready: true, label: '通知已允许', hint: null);
  }
}
