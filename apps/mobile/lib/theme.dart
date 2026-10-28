import 'package:flutter/material.dart';

class HubColors {
  const HubColors(this.dark);
  final bool dark;
  Color get ink => dark ? const Color(0xffe7edf7) : HubTheme.ink;
  Color get muted => dark ? const Color(0xffaebdd2) : HubTheme.muted;
  Color get line => dark ? const Color(0xff34435b) : HubTheme.line;
  Color get accent => dark ? const Color(0xffa2baff) : HubTheme.accent;
  Color get accentDim => dark ? const Color(0xff25365a) : HubTheme.accentDim;
  Color get warn => dark ? const Color(0xfff0be73) : HubTheme.warn;
  Color get warnSurface =>
      dark ? const Color(0xff382e22) : const Color(0xfffff7e9);
  Color get bg2 => dark ? const Color(0xff101722) : HubTheme.bg2;
  Color get surface => dark ? const Color(0xff182232) : Colors.white;
}

class HubTheme {
  static const ink = Color(0xff18243c);
  static const muted = Color(0xff63718a);
  static const line = Color(0xffe3e9f2);
  static const accent = Color(0xff3563e9);
  static const accentDim = Color(0xffedf2ff);
  static const warn = Color(0xff95600a);
  static const bg2 = Color(0xfff6f8fc);

  static HubColors colors(BuildContext context) =>
      HubColors(Theme.of(context).brightness == Brightness.dark);
  static ThemeData light() => _build(Brightness.light);
  static ThemeData dark() => _build(Brightness.dark);
  static ThemeData _build(Brightness brightness) {
    final c = HubColors(brightness == Brightness.dark);
    final ink = c.ink,
        muted = c.muted,
        line = c.line,
        accent = c.accent,
        accentDim = c.accentDim,
        bg2 = c.bg2;
    final base = ColorScheme.fromSeed(
      seedColor: accent,
      brightness: brightness,
      surface: c.surface,
    );
    return ThemeData(
      useMaterial3: true,
      colorScheme: base.copyWith(
        primary: accent,
        onPrimary: c.dark ? c.bg2 : Colors.white,
        surface: c.surface,
        onSurface: ink,
      ),
      scaffoldBackgroundColor: bg2,
      visualDensity: VisualDensity.standard,
      appBarTheme: AppBarTheme(
        backgroundColor: c.surface,
        foregroundColor: ink,
        elevation: 0,
        scrolledUnderElevation: 0,
        toolbarHeight: 68,
        shape: Border(bottom: BorderSide(color: line)),
        centerTitle: false,
      ),
      cardTheme: CardThemeData(
        color: c.surface,
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(16),
          side: BorderSide(color: line),
        ),
        margin: const EdgeInsets.only(bottom: 14),
      ),
      navigationBarTheme: NavigationBarThemeData(
        backgroundColor: c.surface,
        indicatorColor: accentDim,
        height: 72,
        elevation: 0,
        iconTheme: WidgetStateProperty.resolveWith((states) => IconThemeData(
              size: 22,
              color: states.contains(WidgetState.selected) ? accent : muted,
            )),
        labelTextStyle: WidgetStateProperty.resolveWith((states) => TextStyle(
              fontSize: 11,
              fontWeight: states.contains(WidgetState.selected)
                  ? FontWeight.w600
                  : FontWeight.w400,
              color: states.contains(WidgetState.selected) ? accent : muted,
            )),
      ),
      tabBarTheme: TabBarThemeData(
        labelColor: accent,
        unselectedLabelColor: muted,
        indicatorColor: accent,
        dividerColor: line,
        indicatorSize: TabBarIndicatorSize.label,
        tabAlignment: TabAlignment.start,
        labelStyle: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
        unselectedLabelStyle:
            const TextStyle(fontSize: 13, fontWeight: FontWeight.w400),
        labelPadding: const EdgeInsets.symmetric(horizontal: 14),
      ),
      dividerTheme: DividerThemeData(color: line, thickness: 1),
      chipTheme: ChipThemeData(
        backgroundColor: c.surface,
        selectedColor: accentDim,
        side: BorderSide(color: line),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(9)),
        labelStyle: TextStyle(fontSize: 12, color: ink),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          foregroundColor: ink,
          side: BorderSide(color: line),
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 13),
          shape:
              RoundedRectangleBorder(borderRadius: BorderRadius.circular(10)),
        ),
      ),
      bottomSheetTheme: BottomSheetThemeData(
        backgroundColor: c.surface,
        surfaceTintColor: Colors.transparent,
        showDragHandle: true,
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: c.surface,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: line),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: line),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: accent, width: 1.4),
        ),
        labelStyle: TextStyle(color: muted),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          backgroundColor: HubTheme.accent,
          foregroundColor: Colors.white,
          padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 14),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
          ),
        ),
      ),
      textTheme: TextTheme(
        headlineSmall: TextStyle(
          fontSize: 24,
          fontWeight: FontWeight.w600,
          color: ink,
          letterSpacing: -0.4,
        ),
        headlineMedium: TextStyle(
          fontSize: 26,
          fontWeight: FontWeight.w600,
          color: ink,
          letterSpacing: -0.5,
        ),
        titleMedium: TextStyle(
          fontSize: 16,
          fontWeight: FontWeight.w600,
          color: ink,
        ),
        bodyMedium: TextStyle(fontSize: 13, height: 1.65, color: muted),
        labelSmall: TextStyle(fontSize: 11, color: muted),
      ),
    );
  }
}
