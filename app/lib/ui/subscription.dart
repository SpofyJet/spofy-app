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
                    ? _AddForm(model: model, onDone: onDone)
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
  const _AddForm({required this.model, required this.onDone});

  final AppModel model;
  final VoidCallback onDone;

  @override
  State<_AddForm> createState() => _AddFormState();
}

class _AddFormState extends State<_AddForm> {
  final _link = TextEditingController();
  final _focus = FocusNode();
  bool _given = false;

  @override
  void initState() {
    super.initState();
    widget.model.addListener(_offered);
    _offered();
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
    if (text.isNotEmpty) {
      _link.text = text;
      await _submit();
    }
  }

  Future<void> _submit() async {
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
    return ListView(
      padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
      children: [
        Text(_given ? S.offeredHint : S.noSubscriptionHint, style: Ink.note),
        const SizedBox(height: 16),
        Panel(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
          child: Stack(
            children: [
              ValueListenableBuilder<TextEditingValue>(
                valueListenable: _link,
                builder: (context, value, _) => value.text.isEmpty
                    ? Text(S.linkHint, style: Ink.body.copyWith(color: Ink.faint))
                    : const SizedBox.shrink(),
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
                  maxLines: 1,
                  onSubmitted: (_) => _submit(),
                ),
              ),
            ],
          ),
        ),
        if (failure?.task == Task.subscribe) ...[const SizedBox(height: 8), Notice(S.failure(failure!))],
        const SizedBox(height: 8),
        QuietButton(label: S.paste, onTap: busy ? null : _paste),
        const SizedBox(height: 8),
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
    return ListView(
      padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
      children: [
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
        if (failure?.task == Task.subscribe) ...[Notice(S.failure(failure!)), const SizedBox(height: 8)],
        Panel(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (expiry.isNotEmpty) ...[
                Text(S.validUntil, style: Ink.label),
                const SizedBox(height: 4),
                Text(expiry, style: Ink.heading.copyWith(color: left <= 3 ? Ink.warning : Ink.text)),
                const SizedBox(height: 18),
              ],
              Text(S.traffic, style: Ink.label),
              const SizedBox(height: 4),
              Text(S.usage(s), style: Ink.heading),
              if (s.limited) ...[const SizedBox(height: 12), Meter(s.used / s.total)],
            ],
          ),
        ),
        if (failure?.task == Task.refresh) ...[const SizedBox(height: 8), Notice(S.failure(failure!))],
        const SizedBox(height: 8),
        QuietButton(label: S.refresh, busy: model.running(Task.refresh), onTap: model.refresh),
        const SizedBox(height: 8),
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
          QuietButton(label: S.remove, color: Ink.muted, onTap: () => setState(() => _removing = true)),
      ],
    );
  }
}
