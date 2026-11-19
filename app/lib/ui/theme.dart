import 'package:flutter/widgets.dart';

abstract final class Ink {
  static const background = Color(0xFF070D1A);
  static const surface = Color(0xFF101729);
  static const raised = Color(0xFF1A2740);
  static const text = Color(0xFFF3F6FB);
  static const muted = Color(0xFFA4B3CA);
  static const faint = Color(0xFF64748B);
  static const accent = Color(0xFF60A5FA);
  static const onAccent = Color(0xFF07152E);
  static const brand = Color(0xFF2563EB);
  static const selected = Color(0x262563EB);
  static const warning = Color(0xFFFBBF24);
  static const danger = Color(0xFFF87171);
  static const logo = Color(0xFFEF4444);
  static const glow = Color(0x332563EB);

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

  static final title = type(26, 600, text, height: 1.2, spacing: -0.3);
  static final heading = type(20, 600, text, height: 1.25, spacing: -0.2);
  static final body = type(16, 400, text);
  static final strong = type(16, 500, text);
  static final note = type(14, 400, muted, height: 1.35);
  static final label = type(13, 500, muted, spacing: 0.1);
  static final button = type(17, 600, onAccent, height: 1.2);
  static final wordmark = type(22, 700, text, height: 1.1, spacing: -0.4);
}
