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
  });

  final AppModel model;
  final DateTime Function() now;
  final VoidCallback onServers;
  final VoidCallback onSubscription;
  final VoidCallback onSettings;

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
    if (model.subscription == null) {
      return _Empty(model: model, onManual: onSubscription);
    }
    return _Status(model: model, now: now, onServers: onServers, onSubscription: onSubscription);
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
        const SizedBox(height: 16),
        const Center(
          child: DecoratedBox(
            decoration: BoxDecoration(color: Ink.raised, shape: BoxShape.circle),
            child: SizedBox.square(dimension: PowerControl.size),
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
                        const WidgetSpan(
                          alignment: PlaceholderAlignment.middle,
                          child: Padding(
                            padding: EdgeInsets.only(right: 6),
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
        QuietButton(label: S.manual, onTap: widget.onManual),
        const SizedBox(height: 8),
        PrimaryButton(label: S.pasteLink, busy: widget.model.running(Task.subscribe), onTap: _paste),
      ],
    );
  }
}

class _Status extends StatelessWidget {
  const _Status({required this.model, required this.now, required this.onServers, required this.onSubscription});

  final AppModel model;
  final DateTime Function() now;
  final VoidCallback onServers;
  final VoidCallback onSubscription;

  @override
  Widget build(BuildContext context) {
    final connection = model.connection;
    final state = connection.state;
    final current = model.current;
    final chosen = connection.auto ? null : model.snapshot.server(connection.choice);
    final shown = current ?? chosen;
    final hint = S.hint(connection, traffic: model.traffic);
    final failure = model.failure;
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
        const SizedBox(height: 16),
        Center(
          child: PowerControl(state: state, onTap: model.running(Task.disconnect) ? null : model.toggle),
        ),
        const Spacer(flex: 3),
        if (failure != null && failure.task != Task.connect) ...[Notice(S.failure(failure)), const SizedBox(height: 8)],
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
            if (state.busy) const Spinner(size: size, color: Ink.accent, stroke: 4),
          ],
        ),
      ),
    );
  }
}
