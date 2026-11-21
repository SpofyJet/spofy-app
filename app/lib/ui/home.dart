import 'dart:async';

import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/model.dart';
import 'icons.dart';
import 'strings.dart';
import 'theme.dart';
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
      return _Empty(model: model, onManual: onSubscription);
    }
    if (subscription.stopped) {
      return _Stopped(model: model, subscription: subscription, now: now, onSubscription: onSubscription);
    }
    return _Status(model: model, now: now, onServers: onServers, onSubscription: onSubscription, onUpdate: onUpdate);
  }
}

class _Stretch extends StatelessWidget {
  const _Stretch({required this.children});

  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, box) {
        return SingleChildScrollView(
          child: ConstrainedBox(
            constraints: BoxConstraints(minHeight: box.maxHeight),
            child: IntrinsicHeight(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(Ink.gutter, 0, Ink.gutter, 16),
                child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: children),
              ),
            ),
          ),
        );
      },
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
        const Spacer(flex: 2),
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
        const Spacer(flex: 3),
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

class _Empty extends StatefulWidget {
  const _Empty({required this.model, required this.onManual});

  final AppModel model;
  final VoidCallback onManual;

  @override
  State<_Empty> createState() => _EmptyState();
}

class _EmptyState extends State<_Empty> {
  bool _nothing = false;

  Future<void> _paste() async {
    final text = await clipboardText();
    if (!mounted) {
      return;
    }
    if (!text.startsWith('https://') || text.contains(RegExp(r'\s'))) {
      widget.model.dismiss();
      setState(() => _nothing = true);
      return;
    }
    setState(() => _nothing = false);
    await widget.model.subscribe(text);
  }

  @override
  Widget build(BuildContext context) {
    final failure = widget.model.failure;
    final problem = _nothing ? S.noLink : (failure?.task == Task.subscribe ? S.failure(failure!) : '');
    return _Stretch(
      children: [
        const Spacer(flex: 2),
        Text(S.noSubscriptionTitle, style: Ink.title, textAlign: TextAlign.center),
        const SizedBox(height: 8),
        SizedBox(
          height: MediaQuery.textScalerOf(context).scale(Ink.note.fontSize!) * Ink.note.height! * 3,
          child: Semantics(
            liveRegion: true,
            child: problem.isEmpty
                ? Text(
                    S.noSubscriptionHint,
                    style: Ink.note,
                    textAlign: TextAlign.center,
                    maxLines: 3,
                    overflow: TextOverflow.ellipsis,
                  )
                : Text.rich(
                    TextSpan(
                      children: [
                        WidgetSpan(
                          alignment: PlaceholderAlignment.middle,
                          child: Padding(
                            padding: const EdgeInsets.only(right: 6),
                            child: GlyphIcon(Glyph.warning, color: Ink.warning, size: 16),
                          ),
                        ),
                        TextSpan(text: problem),
                      ],
                    ),
                    style: Ink.note.copyWith(color: Ink.text),
                    textAlign: TextAlign.center,
                    maxLines: 3,
                    overflow: TextOverflow.ellipsis,
                  ),
          ),
        ),
        const Spacer(flex: 3),
        if (widget.model.browse case final browse?)
          Pressable(
            label: problem.isEmpty ? '${S.buyAsk} ${S.buy}' : '${S.stuckAsk} ${S.stuck}',
            onTap: () => browse(shopAddress),
            child: ConstrainedBox(
              constraints: const BoxConstraints(minHeight: 44),
              child: Center(
                child: ExcludeSemantics(
                  child: Text.rich(
                    TextSpan(
                      text: '${problem.isEmpty ? S.buyAsk : S.stuckAsk} ',
                      children: [
                        TextSpan(
                          text: problem.isEmpty ? S.buy : S.stuck,
                          style: TextStyle(color: Ink.accent, fontWeight: FontWeight.w600),
                        ),
                      ],
                    ),
                    style: Ink.note,
                    textAlign: TextAlign.center,
                  ),
                ),
              ),
            ),
          ),
        const SizedBox(height: 4),
        QuietButton(label: S.manual, onTap: widget.onManual),
        const SizedBox(height: 8),
        PrimaryButton(label: S.pasteLink, busy: widget.model.running(Task.subscribe), onTap: _paste),
      ],
    );
  }
}

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
              Text(S.stopTitle(subscription), style: Ink.title, textAlign: TextAlign.center),
              const SizedBox(height: 8),
              Text(S.stopHint(subscription), style: Ink.note, textAlign: TextAlign.center),
            ],
          ),
        ),
        const Spacer(flex: 3),
        if (failure?.task == Task.refresh) ...[Notice(S.failure(failure!)), const SizedBox(height: 8)],
        Tile(
          leading: RemainderMark(S.remainder(subscription, now()), low: S.low(subscription, now())),
          title: S.subscription,
          subtitle: S.usage(subscription),
          trailing: subscription.stop == 'ended' ? S.day(subscription, now()) : S.until(subscription, now()),
          trailingColor: S.low(subscription, now()) ? Ink.warning : Ink.muted,
          onTap: onSubscription,
        ),
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
    final shown = current ?? chosen;
    final failure = model.failure;
    final refused = state == VpnState.idle && failure?.task == Task.connect && failure?.code == AppModel.denied;
    final hint = refused ? S.reason(AppModel.denied) : S.hint(connection, traffic: model.traffic);
    final delay = state == VpnState.connected ? connection.delayMs : (shown?.delayMs ?? 0);
    final subscription = model.subscription!;
    return _Stretch(
      children: [
        const Spacer(flex: 2),
        Semantics(
          liveRegion: true,
          child: Column(
            children: [
              SizedBox(
                height: MediaQuery.textScalerOf(context).scale(Ink.title.fontSize!) * Ink.title.height!,
                child: FittedBox(
                  fit: BoxFit.scaleDown,
                  child: Text(S.title(state), style: Ink.title, maxLines: 1),
                ),
              ),
              const SizedBox(height: 6),
              SizedBox(
                height: MediaQuery.textScalerOf(context).scale(Ink.note.fontSize!) * Ink.note.height! * 2,
                child: Text(
                  hint,
                  style: Ink.note,
                  textAlign: TextAlign.center,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
        ),
        SizedBox(
          height: Uptime.slot,
          child: Center(
            child: state.protecting && connection.since > 0 ? Uptime(since: connection.since, now: now) : null,
          ),
        ),
        Center(
          child: PowerControl(state: state, onTap: model.running(Task.disconnect) ? null : model.toggle),
        ),
        const Spacer(flex: 3),
        if (failure != null && failure.task != Task.connect) ...[Notice(S.failure(failure)), const SizedBox(height: 8)],
        if (model.update case final update? when onUpdate != null) ...[
          Pressable(
            label: S.updateBanner(update),
            onTap: onUpdate,
            child: Notice(S.updateBanner(update), color: Ink.accent, glyph: Glyph.refresh),
          ),
          const SizedBox(height: 8),
        ],
        Tile(
          leading: CountryMark(
            shown == null ? 'A' : (shown.country.isEmpty ? '—' : shown.country),
            active: state == VpnState.connected,
          ),
          title: connection.auto ? S.auto : (chosen == null ? connection.choice : S.serverName(chosen)),
          subtitle: connection.auto ? (current == null ? S.autoHint : S.serverName(current)) : S.pinned,
          trailing: delay > 0 ? S.delay(delay) : '',
          onTap: onServers,
          semantic:
              '${S.server}: ${connection.auto ? S.auto : (chosen == null ? connection.choice : S.serverName(chosen))}',
        ),
        const SizedBox(height: 8),
        Tile(
          leading: RemainderMark(S.remainder(subscription, now()), low: S.low(subscription, now())),
          title: S.subscription,
          subtitle: S.usage(subscription),
          trailing: S.until(subscription, now()),
          trailingColor: S.low(subscription, now()) ? Ink.warning : Ink.muted,
          onTap: onSubscription,
        ),
      ],
    );
  }
}

class PowerControl extends StatelessWidget {
  const PowerControl({super.key, required this.state, required this.onTap});

  static const size = 148.0;

  final VpnState state;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final inviting = state == VpnState.idle || state == VpnState.failed;
    final troubled = state == VpnState.stalled || state == VpnState.offline;
    final on = state == VpnState.connected;
    final fill = on ? Ink.accent : Ink.raised;
    final ink = on
        ? Ink.onAccent
        : troubled
        ? Ink.warning
        : inviting
        ? Ink.accent
        : Ink.muted;
    const radius = BorderRadius.all(Radius.circular(size / 2));
    return Pressable(
      label: inviting ? S.connect : (state.busy ? S.stop : S.disconnect),
      onTap: onTap,
      radius: radius,
      child: SizedBox.square(
        dimension: size,
        child: Stack(
          fit: StackFit.expand,
          children: [
            AnimatedContainer(
              duration: const Duration(milliseconds: 180),
              decoration: BoxDecoration(color: fill, shape: BoxShape.circle),
            ),
            Center(child: GlyphIcon(troubled ? Glyph.warning : Glyph.power, color: ink, size: 56, stroke: 1.9)),
            if (state.busy) Spinner(size: size, color: Ink.accent, stroke: 4),
          ],
        ),
      ),
    );
  }
}

class Uptime extends StatefulWidget {
  const Uptime({super.key, required this.since, required this.now});

  static const slot = 68.0;

  final int since;
  final DateTime Function() now;

  @override
  State<Uptime> createState() => _UptimeState();
}

class _UptimeState extends State<Uptime> {
  Timer? _tick;

  @override
  void initState() {
    super.initState();
    _schedule();
  }

  @override
  void dispose() {
    _tick?.cancel();
    super.dispose();
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
    final still = MediaQuery.disableAnimationsOf(context);
    final style = Ink.type(17, 600, Ink.text, height: 1.2).copyWith(fontFeatures: const [FontFeature.tabularFigures()]);
    return Semantics(
      label: '${S.uptimeLabel} $text',
      excludeSemantics: true,
      child: RepaintBoundary(
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: Ink.surface,
            borderRadius: const BorderRadius.all(Radius.circular(999)),
            border: Border.all(color: Ink.good.withValues(alpha: 0.28)),
          ),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(12, 7, 14, 7),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                DecoratedBox(
                  decoration: BoxDecoration(
                    color: Ink.good,
                    shape: BoxShape.circle,
                    boxShadow: [BoxShadow(color: Ink.good.withValues(alpha: 0.55), blurRadius: 8)],
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
        duration: const Duration(milliseconds: 260),
        switchInCurve: Curves.easeOutCubic,
        switchOutCurve: Curves.easeInCubic,
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
