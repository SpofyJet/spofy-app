import 'dart:math' as math;

import 'package:flutter/widgets.dart';

enum Glyph {
  power,
  forward,
  back,
  check,
  warning,
  refresh,
  plus,
  close,
  settings,
  bolt,
  block,
  always,
  apps,
  network,
  bell,
  theme,
  globe,
  chat,
  log,
  search,
  info,
  cut,
  dns,
  route,
  home,
  pulse,
  infinity,
  speed,
}

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
      case Glyph.bolt:
        canvas.drawPath(
          poly(const [
            Offset(13, 3),
            Offset(5, 13.5),
            Offset(11.5, 13.5),
            Offset(10.5, 21),
            Offset(19, 10),
            Offset(12.5, 10),
          ], closed: true),
          line,
        );
      case Glyph.block:
        canvas.drawCircle(const Offset(12, 12), 8.5, line);
        canvas.drawLine(const Offset(6, 6), const Offset(18, 18), line);
      case Glyph.always:
        canvas.drawPath(
          Path()
            ..moveTo(12, 12)
            ..cubicTo(13.5, 9, 15, 7, 16.5, 7)
            ..cubicTo(19.3, 7, 21, 9.5, 21, 12)
            ..cubicTo(21, 14.5, 19.3, 17, 16.5, 17)
            ..cubicTo(15, 17, 13.5, 15, 12, 12)
            ..cubicTo(10.5, 9, 9, 7, 7.5, 7)
            ..cubicTo(4.7, 7, 3, 9.5, 3, 12)
            ..cubicTo(3, 14.5, 4.7, 17, 7.5, 17)
            ..cubicTo(9, 17, 10.5, 15, 12, 12),
          line,
        );
      case Glyph.apps:
        for (final (x, y) in const [(4.0, 4.0), (13.5, 4.0), (4.0, 13.5), (13.5, 13.5)]) {
          canvas.drawRRect(RRect.fromLTRBR(x, y, x + 6.5, y + 6.5, const Radius.circular(1.5)), line);
        }
      case Glyph.network:
        canvas.drawRRect(RRect.fromLTRBR(9, 3, 15, 9, const Radius.circular(1.5)), line);
        canvas.drawRRect(RRect.fromLTRBR(3, 15, 9, 21, const Radius.circular(1.5)), line);
        canvas.drawRRect(RRect.fromLTRBR(15, 15, 21, 21, const Radius.circular(1.5)), line);
        canvas.drawLine(const Offset(12, 9), const Offset(12, 12), line);
        canvas.drawPath(poly(const [Offset(6, 15), Offset(6, 12), Offset(18, 12), Offset(18, 15)]), line);
      case Glyph.bell:
        canvas.drawPath(
          Path()
            ..moveTo(6.5, 17)
            ..lineTo(6.5, 11)
            ..arcToPoint(const Offset(17.5, 11), radius: const Radius.circular(5.5))
            ..lineTo(17.5, 17),
          line,
        );
        canvas.drawLine(const Offset(4.5, 17), const Offset(19.5, 17), line);
        canvas.drawLine(const Offset(12, 3.5), const Offset(12, 5.5), line);
        canvas.drawLine(const Offset(10.5, 20.5), const Offset(13.5, 20.5), line);
      case Glyph.theme:
        canvas.drawCircle(const Offset(12, 12), 8.5, line);
        canvas.drawArc(
          Rect.fromCircle(center: const Offset(12, 12), radius: 8.5),
          -math.pi / 2,
          math.pi,
          true,
          Paint()..color = color,
        );
      case Glyph.globe:
        canvas.drawCircle(const Offset(12, 12), 8.5, line);
        canvas.drawOval(Rect.fromCenter(center: const Offset(12, 12), width: 8, height: 17), line);
        canvas.drawLine(const Offset(3.5, 12), const Offset(20.5, 12), line);
      case Glyph.chat:
        canvas.drawPath(
          Path()
            ..moveTo(7.5, 16.5)
            ..lineTo(7.5, 20.5)
            ..lineTo(12, 16.5)
            ..lineTo(16.5, 16.5)
            ..arcToPoint(const Offset(20.5, 12.5), radius: const Radius.circular(4), clockwise: false)
            ..lineTo(20.5, 8.5)
            ..arcToPoint(const Offset(16.5, 4.5), radius: const Radius.circular(4), clockwise: false)
            ..lineTo(7.5, 4.5)
            ..arcToPoint(const Offset(3.5, 8.5), radius: const Radius.circular(4), clockwise: false)
            ..lineTo(3.5, 12.5)
            ..arcToPoint(const Offset(7.5, 16.5), radius: const Radius.circular(4), clockwise: false)
            ..close(),
          line,
        );
      case Glyph.log:
        canvas.drawRRect(RRect.fromLTRBR(5, 3, 19, 21, const Radius.circular(2.5)), line);
        canvas.drawLine(const Offset(9, 8), const Offset(15, 8), line);
        canvas.drawLine(const Offset(9, 12), const Offset(15, 12), line);
        canvas.drawLine(const Offset(9, 16), const Offset(12.5, 16), line);
      case Glyph.search:
        canvas.drawCircle(const Offset(10.5, 10.5), 6, line);
        canvas.drawLine(const Offset(15, 15), const Offset(20, 20), line);
      case Glyph.info:
        canvas.drawCircle(const Offset(12, 12), 8.5, line);
        canvas.drawLine(const Offset(12, 11), const Offset(12, 16.5), line);
        canvas.drawCircle(const Offset(12, 7.8), 0.4, line);
      case Glyph.cut:
        canvas.drawCircle(const Offset(6.5, 7), 2.5, line);
        canvas.drawCircle(const Offset(6.5, 17), 2.5, line);
        canvas.drawLine(const Offset(8.7, 8.4), const Offset(20, 17), line);
        canvas.drawLine(const Offset(8.7, 15.6), const Offset(20, 7), line);
      case Glyph.dns:
        canvas.drawRRect(RRect.fromLTRBR(3.5, 4, 20.5, 10.5, const Radius.circular(2)), line);
        canvas.drawRRect(RRect.fromLTRBR(3.5, 13.5, 20.5, 20, const Radius.circular(2)), line);
        canvas.drawCircle(const Offset(7.5, 7.25), 0.4, line);
        canvas.drawCircle(const Offset(7.5, 16.75), 0.4, line);
      case Glyph.route:
        canvas.drawCircle(const Offset(6, 5.5), 2.5, line);
        canvas.drawCircle(const Offset(18, 18.5), 2.5, line);
        canvas.drawPath(
          Path()
            ..moveTo(6, 8)
            ..lineTo(6, 9.5)
            ..quadraticBezierTo(6, 12, 8.5, 12)
            ..lineTo(15.5, 12)
            ..quadraticBezierTo(18, 12, 18, 14.5)
            ..lineTo(18, 16),
          line,
        );
      case Glyph.home:
        canvas.drawPath(poly(const [Offset(3.5, 11), Offset(12, 4), Offset(20.5, 11)]), line);
        canvas.drawPath(poly(const [Offset(6, 9.5), Offset(6, 20), Offset(18, 20), Offset(18, 9.5)]), line);
        canvas.drawPath(poly(const [Offset(10, 20), Offset(10, 15), Offset(14, 15), Offset(14, 20)]), line);
      case Glyph.pulse:
        canvas.drawPath(
          poly(const [Offset(3, 12), Offset(7.5, 12), Offset(10, 6), Offset(14, 18), Offset(16.5, 12), Offset(21, 12)]),
          line,
        );
      case Glyph.infinity:
        canvas.drawPath(
          Path()
            ..moveTo(12, 12)
            ..cubicTo(14, 9, 15.5, 8, 17, 8)
            ..cubicTo(19.2, 8, 21, 9.8, 21, 12)
            ..cubicTo(21, 14.2, 19.2, 16, 17, 16)
            ..cubicTo(15.5, 16, 14, 15, 12, 12)
            ..cubicTo(10, 9, 8.5, 8, 7, 8)
            ..cubicTo(4.8, 8, 3, 9.8, 3, 12)
            ..cubicTo(3, 14.2, 4.8, 16, 7, 16)
            ..cubicTo(8.5, 16, 10, 15, 12, 12),
          line,
        );
      case Glyph.speed:
        canvas.drawArc(
          Rect.fromCircle(center: const Offset(12, 13.5), radius: 8.5),
          5 * math.pi / 6,
          4 * math.pi / 3,
          false,
          line,
        );
        canvas.drawLine(const Offset(12, 13.5), const Offset(15.5, 10), line);
        canvas.drawCircle(const Offset(12, 13.5), 0.4, line);
    }
  }

  @override
  bool shouldRepaint(_GlyphPainter old) => old.glyph != glyph || old.color != color || old.stroke != stroke;
}
