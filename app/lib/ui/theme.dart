import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

const themeKey = 'theme';

class Palette {
  const Palette({
    required this.brightness,
    required this.background,
    required this.surface,
    required this.raised,
    required this.text,
    required this.muted,
    required this.faint,
    required this.accent,
    required this.onAccent,
    required this.selected,
    required this.warning,
    required this.good,
    required this.danger,
    required this.glow,
  });

  final Brightness brightness;
  final Color background;
  final Color surface;
  final Color raised;
  final Color text;
  final Color muted;
  final Color faint;
  final Color accent;
  final Color onAccent;
  final Color selected;
  final Color warning;
  final Color good;
  final Color danger;
  final Color glow;

  bool get dark => brightness == Brightness.dark;

  SystemUiOverlayStyle get bars {
    final icons = dark ? Brightness.light : Brightness.dark;
    return SystemUiOverlayStyle(
      statusBarColor: const Color(0x00000000),
      statusBarIconBrightness: icons,
      statusBarBrightness: brightness,
      systemNavigationBarColor: const Color(0x00000000),
      systemNavigationBarIconBrightness: icons,
      systemNavigationBarContrastEnforced: false,
    );
  }
}

Palette paletteFor(Object? setting, Brightness system) => switch (setting) {
  'light' => Ink.light,
  'dark' => Ink.dark,
  _ => system == Brightness.light ? Ink.light : Ink.dark,
};

abstract final class Ink {
  static const dark = Palette(
    brightness: Brightness.dark,
    background: Color(0xFF070D1A),
    surface: Color(0xFF101729),
    raised: Color(0xFF1A2740),
    text: Color(0xFFF3F6FB),
    muted: Color(0xFFA4B3CA),
    faint: Color(0xFF8290A5),
    accent: Color(0xFF60A5FA),
    onAccent: Color(0xFF07152E),
    selected: Color(0x262563EB),
    warning: Color(0xFFFBBF24),
    good: Color(0xFF34D399),
    danger: Color(0xFFF87171),
    glow: Color(0x332563EB),
  );

  static const light = Palette(
    brightness: Brightness.light,
    background: Color(0xFFF3F6FB),
    surface: Color(0xFFFFFFFF),
    raised: Color(0xFFE4EAF3),
    text: Color(0xFF070D1A),
    muted: Color(0xFF475569),
    faint: Color(0xFF5A687D),
    accent: Color(0xFF1D4ED8),
    onAccent: Color(0xFFFFFFFF),
    selected: Color(0x262563EB),
    warning: Color(0xFFA74D08),
    good: Color(0xFF047555),
    danger: Color(0xFFB91C1C),
    glow: Color(0x1A2563EB),
  );

  static Palette _palette = dark;
  static _Type _type = _Type(dark);

  static Palette get palette => _palette;

  static void use(Palette palette) {
    if (palette != _palette) {
      _palette = palette;
      _type = _Type(palette);
    }
  }

  static Color get background => _palette.background;
  static Color get surface => _palette.surface;
  static Color get raised => _palette.raised;
  static Color get text => _palette.text;
  static Color get muted => _palette.muted;
  static Color get faint => _palette.faint;
  static Color get accent => _palette.accent;
  static Color get onAccent => _palette.onAccent;
  static Color get selected => _palette.selected;
  static Color get warning => _palette.warning;
  static Color get good => _palette.good;
  static Color get danger => _palette.danger;
  static Color get glow => _palette.glow;

  static const family = 'Geologica';
  static const gutter = 20.0;
  static const column = 600.0;
  static const radius = BorderRadius.all(Radius.circular(20));

  static TextStyle type(double size, int weight, Color color, {double height = 1.3, double spacing = 0}) => TextStyle(
    fontFamily: family,
    fontSize: size,
    height: height,
    letterSpacing: spacing,
    color: color,
    fontWeight: FontWeight.values[weight ~/ 100 - 1],
    fontVariations: [FontVariation('wght', weight.toDouble())],
    fontFeatures: const [FontFeature.tabularFigures()],
    decoration: TextDecoration.none,
  );

  static TextStyle get title => _type.title;
  static TextStyle get heading => _type.heading;
  static TextStyle get body => _type.body;
  static TextStyle get strong => _type.strong;
  static TextStyle get note => _type.note;
  static TextStyle get label => _type.label;
  static TextStyle get button => _type.button;
  static TextStyle get wordmark => _type.wordmark;
}

class _Type {
  _Type(Palette p)
    : title = Ink.type(26, 600, p.text, height: 1.2, spacing: -0.3),
      heading = Ink.type(20, 600, p.text, height: 1.25, spacing: -0.2),
      body = Ink.type(16, 400, p.text),
      strong = Ink.type(16, 500, p.text),
      note = Ink.type(14, 400, p.muted, height: 1.35),
      label = Ink.type(13, 500, p.muted, spacing: 0.1),
      button = Ink.type(17, 600, p.onAccent, height: 1.2),
      wordmark = Ink.type(22, 700, p.text, height: 1.1, spacing: -0.4);

  final TextStyle title;
  final TextStyle heading;
  final TextStyle body;
  final TextStyle strong;
  final TextStyle note;
  final TextStyle label;
  final TextStyle button;
  final TextStyle wordmark;
}
