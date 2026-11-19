import 'dart:math' as math;

import 'package:flutter/widgets.dart';

enum Glyph { power, forward, back, check, warning, refresh, plus, close, settings }

class GlyphIcon extends StatelessWidget {
  const GlyphIcon(this.glyph, {super.key, required this.color, this.size = 24, this.stroke = 2});

  final Glyph glyph;
  final Color color;
  final double size;
  final double stroke;

  @override
  Widget build(BuildContext context) {
    return ExcludeSemantics(
      child: SizedBox.square(
        dimension: size,
        child: CustomPaint(painter: _GlyphPainter(glyph, color, stroke)),
      ),
    );
  }
}

class _GlyphPainter extends CustomPainter {
  const _GlyphPainter(this.glyph, this.color, this.stroke);

  final Glyph glyph;
  final Color color;
  final double stroke;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.scale(size.width / 24, size.height / 24);
    final line = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;
    Path poly(List<Offset> points, {bool closed = false}) => Path()..addPolygon(points, closed);
    switch (glyph) {
      case Glyph.power:
        canvas.drawArc(
          Rect.fromCircle(center: const Offset(12, 13), radius: 8),
          -math.pi / 2 + 0.62,
          2 * math.pi - 1.24,
          false,
          line,
        );
        canvas.drawLine(const Offset(12, 3.5), const Offset(12, 12), line);
      case Glyph.forward:
        canvas.drawPath(poly(const [Offset(9, 5.5), Offset(15.5, 12), Offset(9, 18.5)]), line);
      case Glyph.back:
        canvas.drawPath(poly(const [Offset(15, 5.5), Offset(8.5, 12), Offset(15, 18.5)]), line);
      case Glyph.check:
        canvas.drawPath(poly(const [Offset(5, 12.5), Offset(10, 17.5), Offset(19, 7)]), line);
      case Glyph.warning:
        canvas.drawPath(poly(const [Offset(12, 4), Offset(21, 19.5), Offset(3, 19.5)], closed: true), line);
        canvas.drawLine(const Offset(12, 10), const Offset(12, 13.5), line);
        canvas.drawCircle(const Offset(12, 16.6), 0.4, line);
      case Glyph.refresh:
        canvas.drawArc(Rect.fromCircle(center: const Offset(12, 12.5), radius: 7.5), 0, 1.5 * math.pi, false, line);
        canvas.drawPath(poly(const [Offset(9.5, 2), Offset(12.5, 5), Offset(9.5, 8)]), line);
      case Glyph.plus:
        canvas.drawLine(const Offset(12, 5), const Offset(12, 19), line);
        canvas.drawLine(const Offset(5, 12), const Offset(19, 12), line);
      case Glyph.close:
        canvas.drawLine(const Offset(6, 6), const Offset(18, 18), line);
        canvas.drawLine(const Offset(18, 6), const Offset(6, 18), line);
      case Glyph.settings:
        canvas.drawLine(const Offset(4, 8), const Offset(11.5, 8), line);
        canvas.drawLine(const Offset(18.5, 8), const Offset(20, 8), line);
        canvas.drawCircle(const Offset(15, 8), 2.5, line);
        canvas.drawLine(const Offset(4, 16), const Offset(5.5, 16), line);
        canvas.drawLine(const Offset(12.5, 16), const Offset(20, 16), line);
        canvas.drawCircle(const Offset(9, 16), 2.5, line);
    }
  }

  @override
  bool shouldRepaint(_GlyphPainter old) => old.glyph != glyph || old.color != color || old.stroke != stroke;
}
