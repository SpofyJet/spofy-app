import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import 'flags.dart';
import 'icons.dart';
import 'motion.dart';
import 'strings.dart';
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
  bool _hovered = false;

  void _press(bool down) {
    if (_down != down) {
      setState(() => _down = down);
    }
  }

  @override
  Widget build(BuildContext context) {
    final enabled = widget.onTap != null;
    return Semantics(
      container: true,
      button: true,
      enabled: enabled,
      label: widget.label,
      excludeSemantics: true,
      child: FocusableActionDetector(
        enabled: enabled,
        mouseCursor: enabled ? SystemMouseCursors.click : MouseCursor.defer,
        onShowFocusHighlight: (focused) => setState(() => _focused = focused),
        onShowHoverHighlight: (hovered) => setState(() => _hovered = hovered),
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
            duration: Motion.of(context, Motion.tap),
            child: DecoratedBox(
              position: DecorationPosition.foreground,
              decoration: BoxDecoration(
                color: _hovered && enabled ? Ink.text.withAlpha(0x0F) : null,
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
  const Panel({super.key, required this.child, this.padding = const EdgeInsets.all(16), this.color});

  final Widget child;
  final EdgeInsets padding;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: BoxDecoration(color: color ?? Ink.surface, borderRadius: Ink.radius),
      child: Padding(padding: padding, child: child),
    );
  }
}

class Toggle extends StatelessWidget {
  const Toggle({super.key, required this.value});

  final bool value;

  @override
  Widget build(BuildContext context) {
    final time = Motion.of(context, Motion.small);
    return AnimatedContainer(
      duration: time,
      width: 48,
      height: 28,
      padding: const EdgeInsets.all(3),
      decoration: BoxDecoration(
        color: value ? Ink.accent : Ink.raised,
        borderRadius: const BorderRadius.all(Radius.circular(14)),
      ),
      child: AnimatedAlign(
        duration: time,
        curve: Motion.move,
        alignment: value ? Alignment.centerRight : Alignment.centerLeft,
        child: AnimatedContainer(
          duration: time,
          width: 22,
          height: 22,
          decoration: BoxDecoration(color: value ? Ink.onAccent : Ink.muted, shape: BoxShape.circle),
        ),
      ),
    );
  }
}

class SwitchTile extends StatelessWidget {
  const SwitchTile({
    super.key,
    required this.title,
    this.subtitle = '',
    this.leading,
    required this.value,
    required this.onChanged,
  });

  final String title;
  final String subtitle;
  final Widget? leading;
  final bool value;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      toggled: value,
      hint: subtitle.isEmpty ? null : subtitle,
      child: Pressable(
        label: title,
        onTap: () => onChanged(!value),
        child: Panel(
          padding: EdgeInsets.symmetric(horizontal: 16, vertical: leading == null ? 14 : 12),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (leading != null) ...[leading!, const SizedBox(width: 12)],
              Expanded(
                child: Padding(
                  padding: EdgeInsets.only(top: leading == null ? 0 : 6),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Expanded(child: Text(title, style: Ink.strong)),
                          const SizedBox(width: 12),
                          Toggle(value: value),
                        ],
                      ),
                      if (subtitle.isNotEmpty)
                        Padding(
                          padding: const EdgeInsets.only(top: 2),
                          child: Text(subtitle, style: Ink.note),
                        ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
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
    this.trailingColor,
    this.wrap = false,
  });

  final String title;
  final String subtitle;
  final Widget? leading;
  final String trailing;
  final VoidCallback? onTap;
  final bool selected;
  final bool arrow;
  final String? semantic;
  final Color? trailingColor;
  final bool wrap;

  @override
  Widget build(BuildContext context) {
    final trailingColor = this.trailingColor ?? Ink.muted;
    final tight = MediaQuery.textScalerOf(context).scale(100) > 115 || MediaQuery.sizeOf(context).width < 340;
    final folded = tight && subtitle.isNotEmpty && trailing.isNotEmpty;
    Widget side(Widget child) => wrap ? SizedBox(height: 40, child: Center(child: child)) : child;
    final row = Panel(
      color: selected ? Color.alphaBlend(Ink.selected, Ink.surface) : Ink.surface,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: ConstrainedBox(
        constraints: const BoxConstraints(minHeight: 40),
        child: Row(
          crossAxisAlignment: wrap ? CrossAxisAlignment.start : CrossAxisAlignment.center,
          children: [
            if (leading != null) ...[leading!, const SizedBox(width: 12)],
            Expanded(
              child: ConstrainedBox(
                constraints: BoxConstraints(minHeight: wrap ? 40 : 0),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisAlignment: MainAxisAlignment.center,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(title, style: Ink.strong, maxLines: wrap ? 2 : 1, overflow: TextOverflow.ellipsis),
                    if (subtitle.isNotEmpty)
                      Padding(
                        padding: const EdgeInsets.only(top: 2),
                        child: wrap || tight
                            ? Folded([
                                ...subtitle.split(' · '),
                                if (folded) trailing,
                              ], last: folded ? trailingColor : null)
                            : Text(subtitle, style: Ink.note, maxLines: 1, overflow: TextOverflow.ellipsis),
                      ),
                  ],
                ),
              ),
            ),
            if (trailing.isNotEmpty && !folded)
              side(
                Padding(
                  padding: const EdgeInsets.only(left: 12),
                  child: Text(trailing, style: Ink.note.copyWith(color: trailingColor)),
                ),
              ),
            if (selected)
              side(
                Padding(
                  padding: const EdgeInsets.only(left: 10),
                  child: GlyphIcon(Glyph.check, color: Ink.accent, size: 20),
                ),
              )
            else if (arrow && onTap != null)
              side(
                Padding(
                  padding: const EdgeInsets.only(left: 6),
                  child: GlyphIcon(Glyph.forward, color: Ink.faint, size: 20),
                ),
              ),
          ],
        ),
      ),
    );
    if (onTap == null) {
      return row;
    }
    return Pressable(
      label: spoken(semantic ?? [title, subtitle, trailing].where((p) => p.isNotEmpty).join(', ')),
      onTap: onTap,
      child: row,
    );
  }
}

String spoken(String text) => text.replaceAll(' · ', ', ').replaceAll('\n', ', ');

class Folded extends StatelessWidget {
  const Folded(this.parts, {super.key, this.style, this.last});

  final List<String> parts;
  final TextStyle? style;
  final Color? last;

  @override
  Widget build(BuildContext context) {
    final style = this.style ?? Ink.note;
    final parts = this.parts.where((p) => p.isNotEmpty).toList();
    return _Fold(
      children: [
        for (var i = 0; i < parts.length; i++) ...[
          if (i > 0) ExcludeSemantics(child: Text(' · ', style: style)),
          Text(parts[i], style: i == parts.length - 1 && last != null ? style.copyWith(color: last) : style),
        ],
      ],
    );
  }
}

class _Fold extends MultiChildRenderObjectWidget {
  const _Fold({required super.children});

  @override
  RenderObject createRenderObject(BuildContext context) => _RenderFold();
}

class _FoldData extends ContainerBoxParentData<RenderBox> {
  bool shown = true;
}

class _RenderFold extends RenderBox
    with ContainerRenderObjectMixin<RenderBox, _FoldData>, RenderBoxContainerDefaultsMixin<RenderBox, _FoldData> {
  @override
  void setupParentData(RenderBox child) {
    if (child.parentData is! _FoldData) {
      child.parentData = _FoldData();
    }
  }

  Size _flow(double width, Size Function(RenderBox child, BoxConstraints constraints) measure, {bool place = false}) {
    var x = 0.0;
    var y = 0.0;
    var line = 0.0;
    var widest = 0.0;
    var open = true;
    var index = 0;
    RenderBox? gap;
    var child = firstChild;
    while (child != null) {
      if (index.isOdd) {
        gap = child;
      } else {
        final natural = child.getMaxIntrinsicWidth(double.infinity);
        if (gap != null) {
          final space = measure(gap, const BoxConstraints()).width;
          final same = open && x + space + natural <= width + 0.01;
          if (place) {
            (gap.parentData! as _FoldData)
              ..shown = same
              ..offset = Offset(x, y);
          }
          if (same) {
            x += space;
          } else {
            y += line;
            x = 0;
            line = 0;
          }
        }
        final room = math.max(0.0, width - x);
        final size = measure(child, BoxConstraints(maxWidth: room));
        if (place) {
          (child.parentData! as _FoldData)
            ..shown = true
            ..offset = Offset(x, y);
        }
        x += size.width;
        widest = math.max(widest, x);
        line = math.max(line, size.height);
        open = natural <= room + 0.01;
        gap = null;
      }
      index++;
      child = childAfter(child);
    }
    return Size(widest, y + line);
  }

  static Size _dry(RenderBox child, BoxConstraints constraints) => child.getDryLayout(constraints);

  static Size _layout(RenderBox child, BoxConstraints constraints) {
    child.layout(constraints, parentUsesSize: true);
    return child.size;
  }

  @override
  Size computeDryLayout(BoxConstraints constraints) => constraints.constrain(_flow(constraints.maxWidth, _dry));

  @override
  void performLayout() {
    size = constraints.constrain(_flow(constraints.maxWidth, _layout, place: true));
  }

  @override
  double computeMinIntrinsicWidth(double height) {
    var widest = 0.0;
    var child = firstChild;
    while (child != null) {
      widest = math.max(widest, child.getMinIntrinsicWidth(double.infinity));
      child = childAfter(child);
    }
    return widest;
  }

  @override
  double computeMaxIntrinsicWidth(double height) {
    var sum = 0.0;
    var child = firstChild;
    while (child != null) {
      sum += child.getMaxIntrinsicWidth(double.infinity);
      child = childAfter(child);
    }
    return sum;
  }

  @override
  double computeMinIntrinsicHeight(double width) => _flow(width, _dry).height;

  @override
  double computeMaxIntrinsicHeight(double width) => _flow(width, _dry).height;

  @override
  double? computeDistanceToActualBaseline(TextBaseline baseline) =>
      defaultComputeDistanceToFirstActualBaseline(baseline);

  @override
  bool hitTestChildren(BoxHitTestResult result, {required Offset position}) =>
      defaultHitTestChildren(result, position: position);

  @override
  void paint(PaintingContext context, Offset offset) {
    var child = firstChild;
    while (child != null) {
      final data = child.parentData! as _FoldData;
      if (data.shown) {
        context.paintChild(child, offset + data.offset);
      }
      child = data.nextSibling;
    }
  }
}

class Balanced extends StatelessWidget {
  const Balanced(this.text, {super.key, required this.style, this.maxLines});

  final String text;
  final TextStyle style;
  final int? maxLines;

  @override
  Widget build(BuildContext context) {
    return Balance(
      child: Text(
        text,
        style: style,
        textAlign: TextAlign.center,
        maxLines: maxLines,
        overflow: maxLines == null ? null : TextOverflow.ellipsis,
      ),
    );
  }
}

class Balance extends SingleChildRenderObjectWidget {
  const Balance({super.key, required super.child});

  @override
  RenderObject createRenderObject(BuildContext context) => _RenderBalance();
}

class _RenderBalance extends RenderShiftedBox {
  _RenderBalance() : super(null);

  @override
  Size computeDryLayout(BoxConstraints constraints) {
    final inner = child?.getDryLayout(constraints.loosen()) ?? Size.zero;
    return constraints.constrain(Size(constraints.hasBoundedWidth ? constraints.maxWidth : inner.width, inner.height));
  }

  @override
  void performLayout() {
    final child = this.child;
    if (child == null) {
      size = constraints.smallest;
      return;
    }
    final loose = constraints.loosen();
    child.layout(loose, parentUsesSize: true);
    final height = child.size.height;
    bool cut() => child is RenderParagraph && child.didExceedMaxLines;
    if (constraints.hasBoundedWidth && !cut() && height > child.getMinIntrinsicHeight(double.infinity) + 0.5) {
      var low = child.getMinIntrinsicWidth(double.infinity);
      var high = loose.maxWidth;
      while (high - low > 1) {
        final middle = (low + high) / 2;
        child.layout(BoxConstraints(maxWidth: middle), parentUsesSize: true);
        if (child.size.height > height + 0.5 || cut()) {
          low = middle;
        } else {
          high = middle;
        }
      }
      child.layout(BoxConstraints(maxWidth: math.min(high + 1, loose.maxWidth)), parentUsesSize: true);
    }
    size = constraints.constrain(
      Size(constraints.hasBoundedWidth ? constraints.maxWidth : child.size.width, child.size.height),
    );
    (child.parentData! as BoxParentData).offset = Offset((size.width - child.size.width) / 2, 0);
  }
}

class Segments<T> extends StatelessWidget {
  const Segments({super.key, required this.options, required this.value, required this.onChanged});

  final List<(T, String)> options;
  final T value;
  final ValueChanged<T> onChanged;

  static const _radius = BorderRadius.all(Radius.circular(16));

  @override
  Widget build(BuildContext context) {
    return Panel(
      padding: const EdgeInsets.all(4),
      child: Row(
        children: [
          for (final (id, label) in options)
            Expanded(
              child: Semantics(
                selected: id == value,
                child: Pressable(
                  label: label,
                  radius: _radius,
                  onTap: () => onChanged(id),
                  child: AnimatedContainer(
                    duration: Motion.of(context, Motion.small),
                    constraints: const BoxConstraints(minHeight: 44),
                    alignment: Alignment.center,
                    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 8),
                    decoration: BoxDecoration(
                      color: id == value ? Ink.raised : Ink.raised.withAlpha(0),
                      borderRadius: _radius,
                    ),
                    child: FittedBox(
                      fit: BoxFit.scaleDown,
                      child: Text(
                        label,
                        style: id == value ? Ink.strong : Ink.strong.copyWith(color: Ink.muted),
                        maxLines: 1,
                      ),
                    ),
                  ),
                ),
              ),
            ),
        ],
      ),
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
        child: ConstrainedBox(
          constraints: const BoxConstraints(minHeight: 56),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Center(
              child: busy
                  ? Spinner(size: 22, color: Ink.onAccent, stroke: 2.5)
                  : Text(
                      label,
                      style: onTap == null ? Ink.button.copyWith(color: Ink.faint) : Ink.button,
                      textAlign: TextAlign.center,
                    ),
            ),
          ),
        ),
      ),
    );
  }
}

class QuietButton extends StatelessWidget {
  const QuietButton({super.key, required this.label, required this.onTap, this.color, this.busy = false});

  final String label;
  final VoidCallback? onTap;
  final Color? color;
  final bool busy;

  @override
  Widget build(BuildContext context) {
    final color = this.color ?? Ink.accent;
    const radius = BorderRadius.all(Radius.circular(14));
    return Pressable(
      label: label,
      onTap: busy ? null : onTap,
      radius: radius,
      child: ConstrainedBox(
        constraints: const BoxConstraints(minHeight: 48),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
          child: Center(
            child: busy
                ? Spinner(size: 20, color: color, stroke: 2.5)
                : Text(
                    label,
                    style: Ink.strong.copyWith(color: onTap == null ? Ink.faint : color),
                    textAlign: TextAlign.center,
                  ),
          ),
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
              ? Spinner(size: 20, color: Ink.muted, stroke: 2.5)
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
    final flag = code.toLowerCase() == 'uk' ? 'gb' : code.toLowerCase();
    if (flagCodes.contains(flag)) {
      return DecoratedBox(
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          border: active ? Border.all(color: Ink.accent, width: 2) : null,
        ),
        child: Padding(
          padding: const EdgeInsets.all(3),
          child: DecoratedBox(
            decoration: BoxDecoration(
              color: Ink.raised,
              shape: BoxShape.circle,
              image: DecorationImage(image: AssetImage('assets/flags/$flag.webp'), fit: BoxFit.cover),
              border: Border.all(color: Ink.faint.withValues(alpha: 0.5)),
            ),
            child: const SizedBox.square(dimension: 34),
          ),
        ),
      );
    }
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

class GlyphMark extends StatelessWidget {
  const GlyphMark(this.glyph, {super.key, this.color});

  final Glyph glyph;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: BoxDecoration(color: Ink.raised, borderRadius: const BorderRadius.all(Radius.circular(12))),
      child: SizedBox.square(
        dimension: 40,
        child: Center(child: GlyphIcon(glyph, color: color ?? Ink.muted, size: 20)),
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
      decoration: BoxDecoration(color: Ink.raised, borderRadius: const BorderRadius.all(Radius.circular(12))),
      child: SizedBox.square(
        dimension: 40,
        child: Center(
          child: SizedBox.square(
            dimension: 20,
            child: TweenAnimationBuilder<double>(
              tween: Tween(end: left.clamp(0.0, 1.0)),
              duration: Motion.of(context, Motion.settle),
              curve: Motion.move,
              builder: (context, left, _) => CustomPaint(painter: _RingPainter(left, low ? Ink.warning : Ink.accent)),
            ),
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
      decoration: BoxDecoration(color: Ink.raised, borderRadius: const BorderRadius.all(Radius.circular(8))),
      child: SizedBox(width: width, height: height),
    );
  }
}

class Meter extends StatelessWidget {
  const Meter(this.share, {super.key, this.color});

  final double share;
  final Color? color;

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
            child: ColoredBox(color: color ?? (value >= 0.9 ? Ink.warning : Ink.accent)),
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
  late final AnimationController _turn = AnimationController(vsync: this, duration: Motion.turn);

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (Motion.still(context)) {
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
          RepaintBoundary(
            child: DecoratedBox(
              decoration: BoxDecoration(
                gradient: RadialGradient(
                  center: const Alignment(0, -1.25),
                  radius: 1.1,
                  colors: [Ink.glow, Ink.background.withAlpha(0)],
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
  const Header({super.key, required this.title, this.onBack, this.action, this.note = ''});

  final String title;
  final VoidCallback? onBack;
  final Widget? action;
  final String note;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 56,
      child: Row(
        children: [
          if (onBack != null)
            Padding(
              padding: const EdgeInsets.only(left: 8),
              child: RoundButton(glyph: Glyph.back, label: S.back, onTap: onBack),
            )
          else
            const SizedBox(width: Ink.gutter),
          Expanded(
            child: Semantics(
              header: true,
              child: Text.rich(
                TextSpan(
                  text: title,
                  children: [
                    if (note.isNotEmpty) ...[
                      const WidgetSpan(
                        alignment: PlaceholderAlignment.baseline,
                        baseline: TextBaseline.alphabetic,
                        child: SizedBox(width: 8),
                      ),
                      TextSpan(
                        text: note,
                        style: Ink.label.copyWith(color: Ink.faint),
                      ),
                    ],
                  ],
                ),
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
  const Notice(this.text, {super.key, this.color, this.glyph = Glyph.warning});

  final String text;
  final Color? color;
  final Glyph glyph;

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
              child: GlyphIcon(glyph, color: color ?? Ink.warning, size: 20),
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

class SearchField extends StatefulWidget {
  const SearchField({super.key, required this.controller, required this.hint, required this.clear, this.focus});

  final TextEditingController controller;
  final String hint;
  final String clear;
  final FocusNode? focus;

  @override
  State<SearchField> createState() => _SearchFieldState();
}

class _SearchFieldState extends State<SearchField> {
  final _field = GlobalKey<EditableTextState>();
  FocusNode? _own;

  FocusNode get _focus => widget.focus ?? (_own ??= FocusNode());

  @override
  void dispose() {
    _own?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      excludeFromSemantics: true,
      onTap: () => _field.currentState?.requestKeyboard(),
      child: Panel(
        padding: const EdgeInsets.fromLTRB(14, 4, 4, 4),
        child: ValueListenableBuilder<TextEditingValue>(
          valueListenable: widget.controller,
          builder: (context, value, _) => Row(
            children: [
              GlyphIcon(Glyph.search, color: Ink.faint, size: 20),
              const SizedBox(width: 10),
              Expanded(
                child: Stack(
                  children: [
                    if (value.text.isEmpty)
                      Text(
                        widget.hint,
                        style: Ink.body.copyWith(color: Ink.faint),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                    Semantics(
                      textField: true,
                      label: widget.hint,
                      child: EditableText(
                        key: _field,
                        controller: widget.controller,
                        focusNode: _focus,
                        style: Ink.body,
                        cursorColor: Ink.accent,
                        backgroundCursorColor: Ink.faint,
                        selectionColor: Ink.selected,
                        textInputAction: TextInputAction.search,
                        autocorrect: false,
                        enableSuggestions: false,
                        maxLines: 1,
                      ),
                    ),
                  ],
                ),
              ),
              if (value.text.isEmpty)
                const SizedBox(height: 44)
              else
                RoundButton(glyph: Glyph.close, label: widget.clear, onTap: widget.controller.clear),
            ],
          ),
        ),
      ),
    );
  }
}

class Selectable extends StatefulWidget {
  const Selectable({super.key, required this.child});

  final Widget child;

  @override
  State<Selectable> createState() => _SelectableState();
}

class _SelectableState extends State<Selectable> {
  final _focus = FocusNode();

  @override
  void dispose() {
    _focus.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final desktop = switch (defaultTargetPlatform) {
      TargetPlatform.windows || TargetPlatform.linux || TargetPlatform.macOS => true,
      _ => false,
    };
    if (!desktop) {
      return widget.child;
    }
    return DefaultSelectionStyle(
      selectionColor: Ink.accent.withAlpha(0x59),
      cursorColor: Ink.accent,
      child: MouseRegion(
        cursor: SystemMouseCursors.text,
        child: SelectableRegion(focusNode: _focus, selectionControls: emptyTextSelectionControls, child: widget.child),
      ),
    );
  }
}

class SectionTitle extends StatelessWidget {
  const SectionTitle(this.text, {super.key});

  final String text;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      header: true,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(4, 20, 4, 8),
        child: Text(text.toUpperCase(), style: Ink.type(12, 600, Ink.faint, spacing: 0.8)),
      ),
    );
  }
}

Color delayColor(int ms) => ms <= 0
    ? Ink.faint
    : ms <= 200
    ? Ink.good
    : ms <= 500
    ? Ink.muted
    : Ink.warning;
