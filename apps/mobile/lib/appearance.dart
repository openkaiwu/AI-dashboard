import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

ThemeMode parseAppearance(String? value) => switch (value) {
      'light' => ThemeMode.light,
      'dark' => ThemeMode.dark,
      _ => ThemeMode.system,
    };

class AppearanceController extends ChangeNotifier {
  AppearanceController({required this.read, required this.write});
  final Future<String?> Function() read;
  final Future<void> Function(String) write;
  ThemeMode mode = ThemeMode.system;
  Future<void> _writes = Future.value();
  int _revision = 0;
  Future<void> load() async {
    final revision = _revision;
    try {
      final stored = await read().timeout(const Duration(seconds: 3));
      if (revision == _revision) {
        mode = parseAppearance(stored);
        notifyListeners();
      }
    } catch (_) {/* Keep system theme when storage is unavailable. */}
  }

  Future<bool> select(ThemeMode value) {
    _revision++;
    mode = value;
    notifyListeners();
    // Serialize writes so rapid changes cannot restore an older selection.
    final saved = _writes.then((_) => write(value.name));
    _writes = saved.catchError((Object _) {});
    return saved.then((_) => true, onError: (Object _) => false);
  }
}

const _storage = FlutterSecureStorage();
final appearance = AppearanceController(
  read: () => _storage.read(key: 'appearance-mode-v1'),
  write: (value) => _storage.write(key: 'appearance-mode-v1', value: value),
);

class AppearanceMenu extends StatelessWidget {
  const AppearanceMenu({super.key});
  @override
  Widget build(BuildContext context) => ListenableBuilder(
      listenable: appearance,
      builder: (context, _) => PopupMenuButton<ThemeMode>(
            tooltip: '界面主题',
            initialValue: appearance.mode,
            icon: Icon(switch (appearance.mode) {
              ThemeMode.system => Icons.brightness_auto_outlined,
              ThemeMode.light => Icons.light_mode_outlined,
              ThemeMode.dark => Icons.dark_mode_outlined,
            }),
            onSelected: (mode) async {
              final saved = await appearance.select(mode);
              if (!saved && context.mounted) {
                ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('已切换主题，本次未能保存偏好。')));
              }
            },
            itemBuilder: (_) => const [
              PopupMenuItem(value: ThemeMode.system, child: Text('跟随系统')),
              PopupMenuItem(value: ThemeMode.light, child: Text('浅色主题')),
              PopupMenuItem(value: ThemeMode.dark, child: Text('深色主题')),
            ],
          ));
}
