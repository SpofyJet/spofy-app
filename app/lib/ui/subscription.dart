import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/model.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

class SubscriptionScreen extends StatelessWidget {
  const SubscriptionScreen({super.key, required this.model, required this.now, required this.onDone});

  final AppModel model;
  final DateTime Function() now;
  final VoidCallback onDone;

  @override
  Widget build(BuildContext context) {
    return Backdrop(
      child: ListenableBuilder(
        listenable: model,
        builder: (context, _) {
          final subscription = model.subscription;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Header(title: S.subscription, onBack: onDone),
              Expanded(
                child: subscription == null
                    ? ListView(
                        padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
                        children: [_AddForm(model: model, onDone: onDone, first: true)],
                      )
                    : _Details(model: model, subscription: subscription, now: now),
              ),
            ],
          );
        },
      ),
    );
  }
}

class _AddForm extends StatefulWidget {
  const _AddForm({required this.model, required this.onDone, this.onCancel, this.first = false});

  final AppModel model;
  final VoidCallback onDone;
  final VoidCallback? onCancel;
  final bool first;

  @override
  State<_AddForm> createState() => _AddFormState();
}

class _AddFormState extends State<_AddForm> {
  final _link = TextEditingController();
  final _focus = FocusNode();
  bool _given = false;
  bool _empty = false;

  @override
  void initState() {
    super.initState();
    if (widget.first) {
      widget.model.addListener(_offered);
      _offered();
    }
    if (!_given) {
      _focus.requestFocus();
    }
  }

  @override
  void dispose() {
    widget.model.removeListener(_offered);
    _link.dispose();
    _focus.dispose();
    super.dispose();
  }

  void _offered() {
    final target = widget.model.ready ? widget.model.take() : null;
    if (target != null) {
      _link.text = target;
      _given = true;
    }
  }

  Future<void> _paste() async {
    final text = await clipboardText();
    if (!mounted) {
      return;
    }
    setState(() => _empty = text.isEmpty);
    if (text.isNotEmpty) {
      _link.text = text;
      await _submit();
    }
  }

  Future<void> _submit() async {
    if (_empty) {
      setState(() => _empty = false);
    }
    if (_link.text.trim().isEmpty) {
      _focus.requestFocus();
      return;
    }
    if (await widget.model.subscribe(_link.text) && mounted) {
      widget.onDone();
    }
  }

  @override
  Widget build(BuildContext context) {
    final model = widget.model;
    final failure = model.failure;
    final busy = model.running(Task.subscribe);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (widget.first) ...[
          Text(_given ? S.offeredHint : S.noSubscriptionHint, style: Ink.note),
          const SizedBox(height: 16),
        ] else
          SectionTitle(S.newSubscription),
        Panel(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
          child: Stack(
            children: [
              ValueListenableBuilder<TextEditingValue>(
                valueListenable: _link,
                builder: (context, value, _) => ExcludeSemantics(
                  child: Visibility(
                    visible: value.text.isEmpty,
                    maintainSize: true,
                    maintainAnimation: true,
                    maintainState: true,
                    child: Text(S.linkHint, style: Ink.body.copyWith(color: Ink.faint)),
                  ),
                ),
              ),
              Semantics(
                textField: true,
                label: S.linkHint,
                child: EditableText(
                  controller: _link,
                  focusNode: _focus,
                  style: Ink.body,
                  cursorColor: Ink.accent,
                  backgroundCursorColor: Ink.faint,
                  selectionColor: Ink.selected,
                  keyboardType: TextInputType.url,
                  textInputAction: TextInputAction.done,
                  autocorrect: false,
                  enableSuggestions: false,
                  minLines: 1,
                  maxLines: 5,
                  onSubmitted: (_) => _submit(),
                ),
              ),
            ],
          ),
        ),
        if (_empty) ...[
          const SizedBox(height: 8),
          Notice(S.noLink),
        ] else if (failure?.task == Task.subscribe) ...[
          const SizedBox(height: 8),
          Notice(S.failure(failure!)),
        ],
        const SizedBox(height: 8),
        QuietButton(label: S.paste, onTap: busy ? null : _paste),
        const SizedBox(height: 8),
        if (widget.onCancel case final cancel?)
          Row(
            children: [
              Expanded(
                child: QuietButton(label: S.cancel, onTap: busy ? null : cancel),
              ),
              Expanded(
                child: QuietButton(label: S.add, color: Ink.accent, busy: busy, onTap: _submit),
              ),
            ],
          )
        else
          PrimaryButton(label: S.add, busy: busy, onTap: _submit),
      ],
    );
  }
}

class _Details extends StatefulWidget {
  const _Details({required this.model, required this.subscription, required this.now});

  final AppModel model;
  final Subscription subscription;
  final DateTime Function() now;

  @override
  State<_Details> createState() => _DetailsState();
}

class _DetailsState extends State<_Details> {
  bool _removing = false;
  bool _adding = false;

  Future<void> _replace() async {
    final target = widget.model.take();
    if (target != null) {
      await widget.model.subscribe(target);
    }
  }

  @override
  Widget build(BuildContext context) {
    final model = widget.model;
    final s = widget.subscription;
    final now = widget.now();
    final failure = model.failure;
    final expiry = S.expiry(s, now);
    final left = S.lifetime(s) || s.expire <= 0 ? 99 : S.daysLeft(s, now);
    final offered = model.offered;
    final page = Uri.tryParse(s.webPageUrl);
    final browse = model.browse;
    final all = model.subscriptions;
    final active = all.where((b) => b.active).firstOrNull;
    final keys = active != null && active.links;
    final switching = model.running(Task.activate);
    return ListView(
      padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
      children: [
        if (all.length > 1) ...[
          SectionTitle(S.subscriptions),
          for (final b in all) ...[
            Tile(
              leading: CountryMark(b.links ? '#' : S.briefTitle(b).characters.first.toUpperCase(), active: b.active),
              title: S.briefTitle(b),
              subtitle: b.active && b.stop.isEmpty ? S.serversCount(b.servers) : S.briefLine(b, now),
              selected: b.active,
              arrow: false,
              wrap: true,
              onTap: b.active || switching ? null : () => model.activate(b.id),
            ),
            const SizedBox(height: 8),
          ],
          if (failure?.task == Task.activate) ...[Notice(S.failure(failure!)), const SizedBox(height: 8)],
          Padding(
            padding: const EdgeInsets.fromLTRB(4, 0, 4, 4),
            child: Text(S.switchHint, style: Ink.type(12, 400, Ink.faint)),
          ),
          SectionTitle(S.chosenSubscription),
        ],
        if (offered != null || model.running(Task.subscribe)) ...[
          Panel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(S.replaceConfirm, style: Ink.strong),
                if (offered != null) ...[
                  const SizedBox(height: 4),
                  Text(S.replaceFrom(offered), style: Ink.note, maxLines: 2, overflow: TextOverflow.ellipsis),
                ],
                const SizedBox(height: 8),
                Row(
                  children: [
                    Expanded(
                      child: QuietButton(label: S.cancel, onTap: offered == null ? null : model.decline),
                    ),
                    Expanded(
                      child: QuietButton(
                        label: S.replace,
                        color: Ink.accent,
                        busy: model.running(Task.subscribe),
                        onTap: _replace,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
          const SizedBox(height: 8),
        ],
        if (failure?.task == Task.subscribe && !_adding) ...[Notice(S.failure(failure!)), const SizedBox(height: 8)],
        if (keys)
          Panel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(S.ownKeys, style: Ink.label),
                const SizedBox(height: 4),
                Text(S.serversCount(active.servers), style: Ink.heading),
              ],
            ),
          )
        else
          Panel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                if (expiry.isNotEmpty) ...[
                  Text(left < 0 ? S.endedOn : S.validUntil, style: Ink.label),
                  const SizedBox(height: 4),
                  Folded(
                    [left < 0 ? S.date(DateTime.fromMillisecondsSinceEpoch(s.expire * 1000), now) : expiry]
                        .expand((e) => e.split(' · '))
                        .toList(),
                    style: Ink.heading.copyWith(color: left <= 3 ? Ink.warning : Ink.text),
                  ),
                  const SizedBox(height: 18),
                ],
                Text(S.traffic, style: Ink.label),
                const SizedBox(height: 4),
                Folded(S.usage(s).split(' · '), style: Ink.heading),
                if (s.limited) ...[const SizedBox(height: 12), Meter(s.used / s.total)],
              ],
            ),
          ),
        if (browse != null && page != null && page.isScheme('https')) ...[
          const SizedBox(height: 8),
          PrimaryButton(label: S.pageAction(s), onTap: () => browse(page.toString())),
        ],
        if (!keys) ...[
          const SizedBox(height: 8),
          QuietButton(label: S.refresh, busy: model.running(Task.refresh), onTap: model.refresh),
          if (S.fetched(s.fetchedAt, now).isNotEmpty)
            Text(S.fetched(s.fetchedAt, now), style: Ink.type(12, 400, Ink.faint), textAlign: TextAlign.center),
        ],
        if (failure?.task == Task.refresh) ...[const SizedBox(height: 8), Notice(S.failure(failure!))],
        const SizedBox(height: 8),
        if (_adding)
          _AddForm(
            model: model,
            onDone: () => setState(() => _adding = false),
            onCancel: () {
              model.dismiss();
              setState(() => _adding = false);
            },
          )
        else
          QuietButton(label: S.addAnother, onTap: () => setState(() => _adding = true)),
        const SizedBox(height: 24),
        if (_removing)
          Panel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(S.removeConfirm, style: Ink.body),
                const SizedBox(height: 8),
                Row(
                  children: [
                    Expanded(
                      child: QuietButton(label: S.cancel, onTap: () => setState(() => _removing = false)),
                    ),
                    Expanded(
                      child: QuietButton(
                        label: S.removeShort,
                        color: Ink.danger,
                        busy: model.running(Task.unsubscribe),
                        onTap: model.unsubscribe,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          )
        else
          QuietButton(label: S.remove, color: Ink.danger, onTap: () => setState(() => _removing = true)),
      ],
    );
  }
}
