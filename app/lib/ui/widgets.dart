import 'dart:math' as math;

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import 'icons.dart';
import 'theme.dart';

Future<String> clipboardText() async {
  try {
    return (await Clipboard.getData(Clipboard.kTextPlain))?.text?.trim() ?? '';
  } on Exception {
    return '';
  }
}

class Pressable extends StatefulWidget {
  const Pressable({super.key, required this.label, required this.onTap, required this.child, this.radius = Ink.radius});

  final String label;
  final VoidCallback? onTap;
  final Widget child;
  final BorderRadius radius;

  @override
  State<Pressable> createState() => _PressableState();
}

class _PressableState extends State<Pressable> {
  bool _down = false;
  bool _focused = false;

  void _press(bool down) {
    if (_down != down) {
      setState(() => _down = down);
    }
  }

  @override
  Widget build(BuildContext context) {
    final enabled = widget.onTap != null;
    return Semantics(
      button: true,
      enabled: enabled,
      label: widget.label,
      excludeSemantics: true,
      child: FocusableActionDetector(
        enabled: enabled,
        mouseCursor: enabled ? SystemMouseCursors.click : MouseCursor.defer,
        onShowFocusHighlight: (focused) => setState(() => _focused = focused),
        actions: {
          ActivateIntent: CallbackAction<ActivateIntent>(
            onInvoke: (_) {
              widget.onTap?.call();
              return null;
            },
          ),
        },
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTapDown: enabled ? (_) => _press(true) : null,
          onTapUp: enabled ? (_) => _press(false) : null,
          onTapCancel: enabled ? () => _press(false) : null,
          onTap: enabled
              ? () {
                  HapticFeedback.selectionClick();
                  widget.onTap!();
                }
              : null,
          child: AnimatedOpacity(
            opacity: _down ? 0.72 : 1,
            duration: const Duration(milliseconds: 90),
            child: DecoratedBox(
              position: DecorationPosition.foreground,
              decoration: BoxDecoration(
                borderRadius: widget.radius,
                border: _focused ? Border.all(color: Ink.accent, width: 2) : null,
              ),
              child: widget.child,
            ),
          ),
        ),
      ),
    );
  }
}

class Panel extends StatelessWidget {
  const Panel({super.key, required this.child, this.padding = const EdgeInsets.all(16), this.color = Ink.surface});

  final Widget child;
  final EdgeInsets padding;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: BoxDecoration(color: color, borderRadius: Ink.radius),
      child: Padding(padding: padding, child: child),
    );
  }
}

class Tile extends StatelessWidget {
  const Tile({
    super.key,
    required this.title,
    this.subtitle = '',
    this.leading,
    this.trailing = '',
    this.onTap,
    this.selected = false,
    this.arrow = true,
    this.semantic,
    this.trailingColor = Ink.muted,
  });

  final String title;
  final String subtitle;
  final Widget? leading;
  final String trailing;
  final VoidCallback? onTap;
  final bool selected;
  final bool arrow;
  final String? semantic;
  final Color trailingColor;

  @override
  Widget build(BuildContext context) {
    final tight = MediaQuery.textScalerOf(context).scale(100) > 115 || MediaQuery.sizeOf(context).width < 340;
    final folded = tight && subtitle.isNotEmpty && trailing.isNotEmpty;
    final row = Panel(
      color: selected ? Color.alphaBlend(Ink.selected, Ink.surface) : Ink.surface,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: ConstrainedBox(
        constraints: const BoxConstraints(minHeight: 40),
        child: Row(
          children: [
            if (leading != null) ...[leading!, const SizedBox(width: 12)],
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(title, style: Ink.strong, maxLines: 1, overflow: TextOverflow.ellipsis),
                  if (subtitle.isNotEmpty)
                    Padding(
                      padding: const EdgeInsets.only(top: 2),
                      child: Text.rich(
                        TextSpan(
                          text: folded ? subtitle.replaceAll(' ', '\u00A0') : subtitle,
                          children: [
                            if (folded) ...[
                              const TextSpan(text: ' · '),
                              TextSpan(
                                text: trailing.replaceAll(' ', '\u00A0'),
                                style: TextStyle(color: trailingColor),
                              ),
                            ],
                          ],
                        ),
                        style: Ink.note,
                        maxLines: tight ? 2 : 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                ],
              ),
            ),
            if (trailing.isNotEmpty && !folded)
              Padding(
                padding: const EdgeInsets.only(left: 12),
                child: Text(trailing, style: Ink.note.copyWith(color: trailingColor)),
              ),
            if (selected)
              const Padding(
                padding: EdgeInsets.only(left: 10),
                child: GlyphIcon(Glyph.check, color: Ink.accent, size: 20),
              )
            else if (arrow && onTap != null)
              const Padding(
                padding: EdgeInsets.only(left: 6),
                child: GlyphIcon(Glyph.forward, color: Ink.faint, size: 20),
              ),
          ],
        ),
      ),
    );
    if (onTap == null) {
      return row;
    }
    return Pressable(
      label: semantic ?? [title, subtitle, trailing].where((p) => p.isNotEmpty).join(', '),
      onTap: onTap,
      child: row,
    );
  }
}

class PrimaryButton extends StatelessWidget {
  const PrimaryButton({super.key, required this.label, required this.onTap, this.busy = false});

  final String label;
  final VoidCallback? onTap;
  final bool busy;

  @override
  Widget build(BuildContext context) {
    final enabled = onTap != null && !busy;
    const radius = BorderRadius.all(Radius.circular(18));
    return Pressable(
      label: label,
      onTap: enabled ? onTap : null,
      radius: radius,
      child: DecoratedBox(
        decoration: BoxDecoration(color: onTap == null ? Ink.raised : Ink.accent, borderRadius: radius),
        child: SizedBox(
          height: 56,
          child: Center(
            child: busy
                ? const Spinner(size: 22, color: Ink.onAccent, stroke: 2.5)
                : Text(label, style: onTap == null ? Ink.button.copyWith(color: Ink.faint) : Ink.button),
          ),
        ),
      ),
    );
  }
}

class QuietButton extends StatelessWidget {
  const QuietButton({super.key, required this.label, required this.onTap, this.color = Ink.accent, this.busy = false});

  final String label;
  final VoidCallback? onTap;
  final Color color;
  final bool busy;

  @override
  Widget build(BuildContext context) {
    const radius = BorderRadius.all(Radius.circular(14));
    return Pressable(
      label: label,
      onTap: busy ? null : onTap,
      radius: radius,
      child: SizedBox(
        height: 48,
        child: Center(
          child: busy
              ? Spinner(size: 20, color: color, stroke: 2.5)
              : Text(label, style: Ink.strong.copyWith(color: onTap == null ? Ink.faint : color)),
        ),
      ),
    );
  }
}

class RoundButton extends StatelessWidget {
  const RoundButton({super.key, required this.glyph, required this.label, required this.onTap, this.busy = false});

  final Glyph glyph;
  final String label;
  final VoidCallback? onTap;
  final bool busy;

  @override
  Widget build(BuildContext context) {
    const radius = BorderRadius.all(Radius.circular(22));
    return Pressable(
      label: label,
      onTap: busy ? null : onTap,
      radius: radius,
      child: SizedBox.square(
        dimension: 44,
        child: Center(
          child: busy
              ? const Spinner(size: 20, color: Ink.muted, stroke: 2.5)
              : GlyphIcon(glyph, color: onTap == null ? Ink.faint : Ink.text),
        ),
      ),
    );
  }
}

class CountryMark extends StatelessWidget {
  const CountryMark(this.code, {super.key, this.active = false});

  final String code;
  final bool active;

  @override
  Widget build(BuildContext context) {
    final text = code.length > 3 ? code.substring(0, 3) : code;
    return DecoratedBox(
      decoration: BoxDecoration(
        color: active ? Color.alphaBlend(Ink.selected, Ink.raised) : Ink.raised,
        borderRadius: const BorderRadius.all(Radius.circular(12)),
      ),
      child: SizedBox.square(
        dimension: 40,
        child: Center(
          child: Text(text.toUpperCase(), style: Ink.type(13, 600, active ? Ink.accent : Ink.muted, spacing: 0.4)),
        ),
      ),
    );
  }
}

class RemainderMark extends StatelessWidget {
  const RemainderMark(this.left, {super.key, this.low = false});

  final double left;
  final bool low;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: const BoxDecoration(color: Ink.raised, borderRadius: BorderRadius.all(Radius.circular(12))),
      child: SizedBox.square(
        dimension: 40,
        child: Center(
          child: SizedBox.square(
            dimension: 20,
            child: CustomPaint(painter: _RingPainter(left.clamp(0.0, 1.0), low ? Ink.warning : Ink.accent)),
          ),
        ),
      ),
    );
  }
}

class _RingPainter extends CustomPainter {
  const _RingPainter(this.left, this.color);

  final double left;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final box = (Offset.zero & size).deflate(1.5);
    final paint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 3
      ..strokeCap = StrokeCap.round
      ..color = Ink.faint.withValues(alpha: 0.45);
    canvas.drawArc(box, 0, 2 * math.pi, false, paint);
    if (left > 0) {
      canvas.drawArc(box, -math.pi / 2, 2 * math.pi * math.max(left, 0.03), false, paint..color = color);
    }
  }

  @override
  bool shouldRepaint(_RingPainter old) => old.left != left || old.color != color;
}

class Skeleton extends StatelessWidget {
  const Skeleton({super.key, required this.width, required this.height});

  final double width;
  final double height;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: const BoxDecoration(color: Ink.raised, borderRadius: BorderRadius.all(Radius.circular(8))),
      child: SizedBox(width: width, height: height),
    );
  }
}

class Meter extends StatelessWidget {
  const Meter(this.share, {super.key});

  final double share;

  @override
  Widget build(BuildContext context) {
    final value = share.clamp(0.0, 1.0);
    return ClipRRect(
      borderRadius: const BorderRadius.all(Radius.circular(4)),
      child: SizedBox(
        height: 8,
        width: double.infinity,
        child: ColoredBox(
          color: Ink.raised,
          child: FractionallySizedBox(
            alignment: Alignment.centerLeft,
            widthFactor: value,
            child: ColoredBox(color: value >= 0.9 ? Ink.warning : Ink.accent),
          ),
        ),
      ),
    );
  }
}

class Spinner extends StatefulWidget {
  const Spinner({super.key, required this.size, required this.color, this.stroke = 3, this.sweep = 1.4});

  final double size;
  final Color color;
  final double stroke;
  final double sweep;

  @override
  State<Spinner> createState() => _SpinnerState();
}

class _SpinnerState extends State<Spinner> with SingleTickerProviderStateMixin {
  late final AnimationController _turn = AnimationController(vsync: this, duration: const Duration(milliseconds: 1100));

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (MediaQuery.disableAnimationsOf(context)) {
      _turn.stop();
    } else if (!_turn.isAnimating) {
      _turn.repeat();
    }
  }

  @override
  void dispose() {
    _turn.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return RepaintBoundary(
      child: RotationTransition(
        turns: _turn,
        child: SizedBox.square(
          dimension: widget.size,
          child: CustomPaint(painter: _ArcPainter(widget.color, widget.stroke, widget.sweep)),
        ),
      ),
    );
  }
}

class _ArcPainter extends CustomPainter {
  const _ArcPainter(this.color, this.stroke, this.sweep);

  final Color color;
  final double stroke;
  final double sweep;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..strokeCap = StrokeCap.round;
    canvas.drawArc((Offset.zero & size).deflate(stroke / 2), -math.pi / 2, sweep, false, paint);
  }

  @override
  bool shouldRepaint(_ArcPainter old) => old.color != color || old.stroke != stroke || old.sweep != sweep;
}

class Backdrop extends StatelessWidget {
  const Backdrop({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return ColoredBox(
      color: Ink.background,
      child: Stack(
        fit: StackFit.expand,
        children: [
          const RepaintBoundary(
            child: DecoratedBox(
              decoration: BoxDecoration(
                gradient: RadialGradient(
                  center: Alignment(0, -1.25),
                  radius: 1.1,
                  colors: [Ink.glow, Color(0x00070D1A)],
                ),
              ),
            ),
          ),
          SafeArea(
            child: Center(
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: Ink.column),
                child: child,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class Header extends StatelessWidget {
  const Header({super.key, required this.title, this.onBack, this.action});

  final String title;
  final VoidCallback? onBack;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 56,
      child: Row(
        children: [
          if (onBack != null)
            Padding(
              padding: const EdgeInsets.only(left: 8),
              child: RoundButton(glyph: Glyph.back, label: 'Назад', onTap: onBack),
            )
          else
            const SizedBox(width: Ink.gutter),
          Expanded(
            child: Semantics(
              header: true,
              child: Text(
                title,
                style: onBack == null ? Ink.wordmark : Ink.heading,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ),
          if (action != null)
            Padding(padding: const EdgeInsets.only(right: 8), child: action)
          else
            const SizedBox(width: Ink.gutter),
        ],
      ),
    );
  }
}

class Notice extends StatelessWidget {
  const Notice(this.text, {super.key, this.color = Ink.warning});

  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      liveRegion: true,
      child: Panel(
        color: Ink.raised,
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.only(top: 1),
              child: GlyphIcon(Glyph.warning, color: color, size: 20),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Text(text, style: Ink.note.copyWith(color: Ink.text)),
            ),
          ],
        ),
      ),
    );
  }
}
