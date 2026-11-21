import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/model.dart';
import 'icons.dart';
import 'motion.dart';
import 'strings.dart';
import 'theme.dart';
import 'welcome.dart';
import 'widgets.dart';

class HomeScreen extends StatelessWidget {
  const HomeScreen({
    super.key,
    required this.model,
    required this.now,
    required this.onServers,
    required this.onSubscription,
    required this.onSettings,
    this.onUpdate,
  });

  final AppModel model;
  final DateTime Function() now;
  final VoidCallback onServers;
  final VoidCallback onSubscription;
  final VoidCallback onSettings;
  final VoidCallback? onUpdate;

  @override
  Widget build(BuildContext context) {
    return Backdrop(
      child: ListenableBuilder(
        listenable: model,
        builder: (context, _) {
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Header(
                title: S.app,
                note: model.ready ? model.snapshot.version : '',
                action: model.ready ? RoundButton(glyph: Glyph.settings, label: S.settings, onTap: onSettings) : null,
              ),
              if (model.fault.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 0),
                  child: Notice(
                    model.fault == AppModel.coreProtocol ? S.coreProtocol : S.coreClosed,
                    color: Ink.danger,
                  ),
                ),
              if (model.fault == AppModel.coreClosed && model.reopen != null)
                Padding(
                  padding: const EdgeInsets.fromLTRB(Ink.gutter, 8, Ink.gutter, 0),
                  child: QuietButton(label: S.tryAgain, busy: model.reviving, onTap: model.revive),
                ),
              Expanded(child: _body()),
            ],
          );
        },
      ),
    );
  }

  Widget _body() {
    if (!model.ready) {
      return model.fault.isEmpty ? const _Loading() : const SizedBox.shrink();
    }
    final subscription = model.subscription;
    if (subscription == null) {
      return Welcome(model: model, onManual: onSubscription);
    }
    if (subscription.stopped) {
      return _Stopped(model: model, subscription: subscription, now: now, onSubscription: onSubscription);
    }
    return _Status(model: model, now: now, onServers: onServers, onSubscription: onSubscription, onUpdate: onUpdate);
  }
}

Server? shownServer(AppModel model) {
  final subscription = model.subscription;
  if (subscription == null || subscription.stopped) {
    return null;
  }
  final connection = model.connection;
  return model.current ?? (connection.auto ? null : model.snapshot.server(connection.choice));
}

class _Stretch extends StatelessWidget {
  const _Stretch({required this.children});

  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    return CustomScrollView(
      slivers: [
        SliverFillRemaining(
          hasScrollBody: false,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(Ink.gutter, 0, Ink.gutter, 16),
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: children),
          ),
        ),
      ],
    );
  }
}

class _Loading extends StatelessWidget {
  const _Loading();

  @override
  Widget build(BuildContext context) {
    final tile = Panel(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: ConstrainedBox(
        constraints: const BoxConstraints(minHeight: 40),
        child: _Hidden(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(' ', style: Ink.strong, maxLines: 1),
              Padding(
                padding: const EdgeInsets.only(top: 2),
                child: Text(' ', style: Ink.note, maxLines: 1),
              ),
            ],
          ),
        ),
      ),
    );
    return _Stretch(
      children: [
        const Spacer(),
        Stack(
          alignment: Alignment.center,
          children: [
            SizedBox(height: MediaQuery.textScalerOf(context).scale(Ink.title.fontSize!) * Ink.title.height!),
            const Skeleton(width: 190, height: 24),
          ],
        ),
        const SizedBox(height: 6),
        SizedBox(
          height: MediaQuery.textScalerOf(context).scale(Ink.note.fontSize!) * Ink.note.height! * 2,
          child: const Align(alignment: Alignment.topCenter, child: Skeleton(width: 170, height: 16)),
        ),
        const SizedBox(height: Uptime.slot),
        Center(
          child: DecoratedBox(
            decoration: BoxDecoration(color: Ink.raised, shape: BoxShape.circle),
            child: const SizedBox.square(dimension: PowerControl.size),
          ),
        ),
        SizedBox(height: Speed.slot(context)),
        const Expanded(child: _Notices([])),
        tile,
        const SizedBox(height: 8),
        tile,
      ],
    );
  }
}

class _Hidden extends StatelessWidget {
  const _Hidden({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) => ExcludeSemantics(child: Opacity(opacity: 0, child: child));
}

const shopAddress = 'https://t.me/spofyvpnbot';

class _Stopped extends StatelessWidget {
  const _Stopped({required this.model, required this.subscription, required this.now, required this.onSubscription});

  final AppModel model;
  final Subscription subscription;
  final DateTime Function() now;
  final VoidCallback onSubscription;

  @override
  Widget build(BuildContext context) {
    final failure = model.failure;
    final page = Uri.tryParse(subscription.webPageUrl);
    final browse = model.browse;
    return _Stretch(
      children: [
        const Spacer(flex: 2),
        Center(child: GlyphIcon(Glyph.warning, color: Ink.warning, size: 40, stroke: 1.9)),
        const SizedBox(height: 16),
        Semantics(
          liveRegion: true,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Balanced(S.stopTitle(subscription), style: Ink.title),
              const SizedBox(height: 8),
              Balanced(S.stopHint(subscription), style: Ink.note),
            ],
          ),
        ),
        Expanded(flex: 3, child: _Notices([if (failure?.task == Task.refresh) Notice(S.failure(failure!))])),
        _SubscriptionTile(model: model, subscription: subscription, now: now, onTap: onSubscription),
        const SizedBox(height: 8),
        QuietButton(label: S.checkAgain, busy: model.running(Task.refresh), onTap: model.refresh),
        if (browse != null && page != null && page.isScheme('https')) ...[
          const SizedBox(height: 8),
          PrimaryButton(label: S.stopAction(subscription), onTap: () => browse(page.toString())),
        ],
      ],
    );
  }
}

class _Status extends StatelessWidget {
  const _Status({
    required this.model,
    required this.now,
    required this.onServers,
    required this.onSubscription,
    this.onUpdate,
  });

  final AppModel model;
  final DateTime Function() now;
  final VoidCallback onServers;
  final VoidCallback onSubscription;
  final VoidCallback? onUpdate;

  @override
  Widget build(BuildContext context) {
    final connection = model.connection;
    final state = connection.state;
    final current = model.current;
    final chosen = connection.auto ? null : model.snapshot.server(connection.choice);
    final shown = shownServer(model);
    final failure = model.failure;
    final refused = state == VpnState.idle && failure?.task == Task.connect && failure?.code == AppModel.denied;
    final hint = refused ? S.reason(AppModel.denied) : S.hint(connection, traffic: model.traffic);
    final delay = state == VpnState.connected ? connection.delayMs : (shown?.delayMs ?? 0);
    final subscription = model.subscription!;
    final totals = state == VpnState.connected && !refused && (model.traffic.down > 0 || model.traffic.up > 0);
    return _Stretch(
      children: [
        const Spacer(),
        Semantics(
          liveRegion: true,
          child: Column(
            children: [
              SizedBox(
                height: MediaQuery.textScalerOf(context).scale(Ink.title.fontSize!) * Ink.title.height!,
                child: _Swap(
                  alignment: Alignment.center,
                  child: FittedBox(
                    key: ValueKey(state),
                    fit: BoxFit.scaleDown,
                    child: Text(S.title(state), style: Ink.title, maxLines: 1),
                  ),
                ),
              ),
              const SizedBox(height: 6),
              SizedBox(
                height: MediaQuery.textScalerOf(context).scale(Ink.note.fontSize!) * Ink.note.height! * 2,
                child: _Swap(
                  child: Balanced(hint, key: ValueKey<String?>(totals ? null : hint), style: Ink.note, maxLines: 2),
                ),
              ),
            ],
          ),
        ),
        RepaintBoundary(
          child: SizedBox(
            height: Uptime.slot,
            child: Center(
              child: _Swap(
                alignment: Alignment.center,
                child: state.protecting && connection.since > 0
                    ? Uptime(
                        since: connection.since,
                        now: now,
                        tone: switch (state) {
                          VpnState.connected => Ink.good,
                          VpnState.switching => Ink.faint,
                          _ => Ink.warning,
                        },
                      )
                    : null,
              ),
            ),
          ),
        ),
        Center(
          child: PowerControl(state: state, onTap: model.running(Task.disconnect) ? null : model.toggle),
        ),
        RepaintBoundary(
          child: SizedBox(
            height: Speed.slot(context),
            child: _Swap(child: state == VpnState.connected ? Speed(rate: model.speed) : null),
          ),
        ),
        Expanded(
          child: _Notices([
            if (failure?.task == Task.disconnect) Notice(S.failure(failure!)),
            if (model.update case final update? when onUpdate != null)
              Pressable(
                label: S.updateBanner(update),
                onTap: onUpdate,
                child: Notice(S.updateBanner(update), color: Ink.accent, glyph: Glyph.refresh),
              ),
          ]),
        ),
        Tile(
          leading: shown == null
              ? GlyphMark(Glyph.bolt, color: Ink.accent)
              : CountryMark(shown.country.isEmpty ? '—' : shown.country, active: state == VpnState.connected),
          title: connection.auto ? S.auto : (chosen == null ? connection.choice : S.place(chosen)),
          subtitle: connection.auto ? (current == null ? S.autoHint : S.place(current)) : S.pinned,
          trailing: delay > 0 ? S.delay(delay) : '',
          onTap: onServers,
          semantic: '${S.server}: ${connection.auto ? S.auto : (chosen == null ? connection.choice : S.place(chosen))}',
        ),
        const SizedBox(height: 8),
        _SubscriptionTile(model: model, subscription: subscription, now: now, onTap: onSubscription),
      ],
    );
  }
}

class _Notices extends StatelessWidget {
  const _Notices(this.children);

  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: Alignment.bottomCenter,
      child: Padding(
        padding: const EdgeInsets.only(top: 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [for (final child in children) Padding(padding: const EdgeInsets.only(bottom: 8), child: child)],
        ),
      ),
    );
  }
}

class _SubscriptionTile extends StatelessWidget {
  const _SubscriptionTile({required this.model, required this.subscription, required this.now, required this.onTap});

  final AppModel model;
  final Subscription subscription;
  final DateTime Function() now;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final s = subscription;
    final today = now();
    final brief = model.subscriptions.where((b) => b.active).firstOrNull;
    final keys = brief != null && brief.links;
    final endless = S.endless(s);
    final low = S.low(s, today);
    final title = brief != null && (keys || model.subscriptions.length > 1) ? S.briefTitle(brief) : S.subscription;
    final subtitle = keys ? S.serversCount(brief.servers) : S.plan(s);
    final trailing = keys || endless ? '' : (s.stop == 'ended' ? S.day(s, today) : S.until(s, today));
    return Tile(
      leading: keys
          ? CountryMark('#')
          : endless
          ? GlyphMark(Glyph.infinity, color: Ink.accent)
          : RemainderMark(S.remainder(s, today), low: low),
      title: title,
      subtitle: subtitle,
      trailing: trailing,
      trailingColor: low ? Ink.warning : Ink.muted,
      wrap: endless,
      onTap: onTap,
      semantic: [
        if (title != S.subscription) '${S.subscription}: $title' else title,
        subtitle,
        trailing,
      ].where((p) => p.isNotEmpty).join(', '),
    );
  }
}

class _Swap extends StatelessWidget {
  const _Swap({required this.child, this.alignment = Alignment.topCenter});

  final Widget? child;
  final AlignmentGeometry alignment;

  @override
  Widget build(BuildContext context) {
    return AnimatedSwitcher(
      duration: Motion.of(context, Motion.medium),
      switchInCurve: Motion.enter,
      switchOutCurve: Motion.exit,
      layoutBuilder: (current, previous) => Stack(alignment: alignment, children: [...previous, ?current]),
      transitionBuilder: (child, animation) => AnimatedBuilder(
        animation: animation,
        builder: (context, faded) =>
            ExcludeSemantics(excluding: animation.status == AnimationStatus.reverse, child: faded),
        child: FadeTransition(opacity: animation, child: child),
      ),
      child: child,
    );
  }
}

class PowerControl extends StatefulWidget {
  const PowerControl({super.key, required this.state, required this.onTap});

  static const size = 148.0;

  final VpnState state;
  final VoidCallback? onTap;

  @override
  State<PowerControl> createState() => _PowerControlState();
}

class _PowerControlState extends State<PowerControl> with TickerProviderStateMixin {
  late final _turn = AnimationController(vsync: this, duration: Motion.turn);
  late final _ring = AnimationController(vsync: this, duration: Motion.medium, value: widget.state.busy ? 1 : 0)
    ..addStatusListener((status) {
      if (status.isDismissed) {
        _turn.stop();
      }
    });
  late final _lit = AnimationController(vsync: this, duration: Motion.medium, value: _on ? 1 : 0);
  late final _seal = AnimationController(vsync: this, duration: Motion.settle, value: 1)
    ..addStatusListener((status) {
      if (status.isCompleted && !widget.state.busy) {
        _ring.value = 0;
      }
    });
  late final _shine = Listenable.merge([_lit, _seal]);
  late final _spin = Listenable.merge([_turn, _ring, _seal]);

  bool get _on => widget.state == VpnState.connected;

  bool get _sealing => _seal.value < 1;

  double get _fill => _sealing ? _part(_seal.value, 0.3, 0.65, Motion.move) : _lit.value;

  double get _halo => _sealing ? _part(_seal.value, 0.35, 1, Motion.enter) : _lit.value;

  double get _close => _sealing ? _part(_seal.value, 0, 0.4, Motion.enter) : 0;

  double get _shown => _ring.value * (_sealing ? 1 - _part(_seal.value, 0.45, 0.75, Curves.linear) : 1);

  static double _part(double t, double from, double to, Curve curve) =>
      curve.transform(((t - from) / (to - from)).clamp(0.0, 1.0));

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _run(null);
  }

  @override
  void didUpdateWidget(PowerControl old) {
    super.didUpdateWidget(old);
    if (old.state != widget.state) {
      _run(old.state);
    }
  }

  void _run(VpnState? was) {
    final busy = widget.state.busy;
    if (Motion.still(context)) {
      _turn.stop();
      _seal.value = 1;
      _ring.value = busy ? 1 : 0;
      _lit.value = _on ? 1 : 0;
      return;
    }
    if (busy && !_turn.isAnimating) {
      _turn.repeat();
    }
    if (_on && was != null && was.busy) {
      _seal.forward(from: 0);
    } else {
      _seal.value = 1;
      if (busy) {
        _ring.forward();
      } else {
        _ring.reverse();
      }
    }
    _lit.animateTo(_on ? 1 : 0, curve: Motion.move);
  }

  @override
  void dispose() {
    _turn.dispose();
    _ring.dispose();
    _lit.dispose();
    _seal.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.state;
    final inviting = state == VpnState.idle || state == VpnState.failed;
    final troubled = state == VpnState.stalled || state == VpnState.offline;
    final ink = troubled
        ? Ink.warning
        : inviting
        ? Ink.accent
        : Ink.muted;
    const radius = BorderRadius.all(Radius.circular(PowerControl.size / 2));
    return Pressable(
      label: inviting ? S.connect : (state.busy ? S.stop : S.disconnect),
      onTap: widget.onTap,
      radius: radius,
      child: RepaintBoundary(
        child: SizedBox.square(
          dimension: PowerControl.size,
          child: Stack(
            fit: StackFit.expand,
            children: [
              CustomPaint(painter: _Disc(this, Ink.raised, Ink.accent)),
              Center(
                child: TweenAnimationBuilder<Color?>(
                  tween: ColorTween(end: ink),
                  duration: Motion.of(context, Motion.medium),
                  builder: (context, color, _) => AnimatedBuilder(
                    animation: _shine,
                    builder: (context, _) => _Swap(
                      alignment: Alignment.center,
                      child: GlyphIcon(
                        troubled ? Glyph.warning : Glyph.power,
                        key: ValueKey(troubled),
                        color: Color.lerp(color ?? ink, Ink.onAccent, _fill)!,
                        size: 56,
                        stroke: 1.9,
                      ),
                    ),
                  ),
                ),
              ),
              RepaintBoundary(child: CustomPaint(painter: _Ring(this, Ink.accent))),
            ],
          ),
        ),
      ),
    );
  }
}

class _Disc extends CustomPainter {
  _Disc(this.control, this.off, this.on) : super(repaint: control._shine);

  final _PowerControlState control;
  final Color off;
  final Color on;

  @override
  void paint(Canvas canvas, Size size) {
    final center = size.center(Offset.zero);
    final radius = size.shortestSide / 2;
    final halo = control._halo;
    if (halo > 0) {
      canvas.drawCircle(
        center,
        radius + 4 * halo,
        BoxShadow(color: on.withAlpha((0x59 * halo).round()), blurRadius: 40).toPaint(),
      );
    }
    canvas.drawCircle(center, radius, Paint()..color = Color.lerp(off, on, control._fill)!);
  }

  @override
  bool shouldRepaint(_Disc old) => old.off != off || old.on != on || old.control != control;
}

class _Ring extends CustomPainter {
  _Ring(this.control, this.color) : super(repaint: control._spin);

  final _PowerControlState control;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final shown = control._shown;
    if (shown <= 0) {
      return;
    }
    const stroke = 4.0;
    final center = size.center(Offset.zero);
    canvas.transform(
      (Matrix4.translationValues(center.dx, center.dy, 0)
            ..multiply(Matrix4.rotationZ(control._turn.value * math.pi * 2))
            ..multiply(Matrix4.translationValues(-center.dx, -center.dy, 0)))
          .storage,
    );
    canvas.drawArc(
      (Offset.zero & size).deflate(stroke / 2),
      -math.pi / 2,
      1.4 + (2 * math.pi - 1.4) * control._close,
      false,
      Paint()
        ..color = color.withValues(alpha: color.a * shown)
        ..style = PaintingStyle.stroke
        ..strokeWidth = stroke
        ..strokeCap = StrokeCap.round,
    );
  }

  @override
  bool shouldRepaint(_Ring old) => old.color != color || old.control != control;
}

class Speed extends StatelessWidget {
  const Speed({super.key, required this.rate});

  static double slot(BuildContext context) => 16 + MediaQuery.textScalerOf(context).scale(15) * 1.2;

  final Traffic? rate;

  @override
  Widget build(BuildContext context) {
    final rate = this.rate;
    final style = Ink.type(15, 600, Ink.text, height: 1.2);
    Widget side(String arrow, int? value) => FittedBox(
      fit: BoxFit.scaleDown,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(arrow, style: style.copyWith(color: Ink.faint)),
          const SizedBox(width: 4),
          _Swap(
            alignment: Alignment.centerLeft,
            child: value == null ? const Skeleton(width: 72, height: 16) : Text(S.rate(value), style: style),
          ),
        ],
      ),
    );
    return Semantics(
      label: rate == null ? '' : S.speed(rate),
      excludeSemantics: true,
      child: Padding(
        padding: const EdgeInsets.only(top: 16),
        child: Row(
          children: [
            Expanded(
              child: Align(alignment: Alignment.centerRight, child: side('↓', rate?.down)),
            ),
            const SizedBox(width: 24),
            Expanded(
              child: Align(alignment: Alignment.centerLeft, child: side('↑', rate?.up)),
            ),
          ],
        ),
      ),
    );
  }
}

class Uptime extends StatefulWidget {
  const Uptime({super.key, required this.since, required this.now, this.tone});

  static const slot = 68.0;

  final int since;
  final DateTime Function() now;
  final Color? tone;

  @override
  State<Uptime> createState() => _UptimeState();
}

class _UptimeState extends State<Uptime> with WidgetsBindingObserver {
  Timer? _tick;
  bool _covered = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) => setState(_wake);

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _covered = !TickerMode.valuesOf(context).enabled;
    _wake();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _tick?.cancel();
    super.dispose();
  }

  void _wake() {
    final life = WidgetsBinding.instance.lifecycleState;
    final live = !_covered && life != AppLifecycleState.hidden && life != AppLifecycleState.paused;
    if (live == (_tick != null)) {
      return;
    }
    _tick?.cancel();
    _tick = null;
    if (live) {
      _schedule();
    }
  }

  Duration get _elapsed => widget.now().difference(DateTime.fromMillisecondsSinceEpoch(widget.since * 1000));

  void _schedule() {
    final left = 1000 - _elapsed.inMilliseconds % 1000;
    _tick = Timer(Duration(milliseconds: left <= 0 ? 1000 : left), () {
      if (mounted) {
        setState(_schedule);
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final elapsed = _elapsed;
    final text = S.uptime(elapsed);
    final still = Motion.still(context);
    final style = Ink.type(17, 600, Ink.text, height: 1.2).copyWith(fontFeatures: const [FontFeature.tabularFigures()]);
    final tone = widget.tone ?? Ink.good;
    return Semantics(
      label: '${S.uptimeLabel} $text',
      excludeSemantics: true,
      child: RepaintBoundary(
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: Ink.surface,
            borderRadius: const BorderRadius.all(Radius.circular(999)),
            border: Border.all(color: tone.withValues(alpha: 0.28)),
          ),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(12, 7, 14, 7),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                DecoratedBox(
                  decoration: BoxDecoration(
                    color: tone,
                    shape: BoxShape.circle,
                    boxShadow: [BoxShadow(color: tone.withValues(alpha: 0.55), blurRadius: 8)],
                  ),
                  child: const SizedBox.square(dimension: 8),
                ),
                const SizedBox(width: 10),
                for (var i = 0; i < text.length; i++)
                  _Digit(text[i], key: ValueKey(text.length - i), style: style, still: still),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _Digit extends StatelessWidget {
  const _Digit(this.char, {super.key, required this.style, required this.still});

  final String char;
  final TextStyle style;
  final bool still;

  @override
  Widget build(BuildContext context) {
    final glyph = Text(
      char,
      key: ValueKey(char),
      style: char == ':' ? style.copyWith(color: Ink.faint) : style,
    );
    if (still || !RegExp(r'\d').hasMatch(char)) {
      return glyph;
    }
    return ClipRect(
      child: AnimatedSwitcher(
        duration: Motion.large,
        switchInCurve: Motion.enter,
        switchOutCurve: Motion.exit,
        layoutBuilder: (current, previous) => Stack(alignment: Alignment.center, children: [...previous, ?current]),
        transitionBuilder: (child, animation) {
          final entering = child.key == ValueKey(char);
          final offset = Tween<Offset>(
            begin: entering ? const Offset(0, 0.6) : const Offset(0, -0.6),
            end: Offset.zero,
          ).animate(animation);
          return FadeTransition(
            opacity: animation,
            child: SlideTransition(position: offset, child: child),
          );
        },
        child: glyph,
      ),
    );
  }
}
