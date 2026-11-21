import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import '../state/model.dart';
import 'settings.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

class Choice {
  const Choice(this.value, this.title, [this.hint = '']);

  final Object? value;
  final String title;
  final String hint;
}

class ChoiceScreen extends StatefulWidget {
  const ChoiceScreen({
    super.key,
    required this.model,
    required this.title,
    required this.setting,
    required this.choices,
    required this.onDone,
    this.own,
    this.ownHint = '',
    this.hint = '',
  });

  final AppModel model;
  final String title;
  final String setting;
  final List<Choice> choices;
  final VoidCallback onDone;
  final String? own;
  final String ownHint;
  final String hint;

  @override
  State<ChoiceScreen> createState() => _ChoiceScreenState();
}

class _ChoiceScreenState extends State<ChoiceScreen> {
  final _text = TextEditingController();
  final _focus = FocusNode();
  bool _typing = false;
  bool _refused = false;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    final value = widget.model.snapshot.settings[widget.setting];
    if (value is String && !widget.choices.any((c) => c.value == value)) {
      _text.text = value;
    }
  }

  @override
  void dispose() {
    _text.dispose();
    _focus.dispose();
    super.dispose();
  }

  Future<void> _pick(Object? value) async {
    setState(() {
      _typing = false;
      _refused = false;
    });
    await widget.model.set(widget.setting, value);
  }

  Future<void> _save() async {
    final text = _text.text.trim();
    if (text.isEmpty) {
      _focus.requestFocus();
      return;
    }
    setState(() => _saving = true);
    final ok = await widget.model.set(widget.setting, text);
    if (!mounted) {
      return;
    }
    setState(() {
      _saving = false;
      _refused = !ok;
      _typing = !ok;
    });
  }

  @override
  Widget build(BuildContext context) {
    return Backdrop(
      child: ListenableBuilder(
        listenable: widget.model,
        builder: (context, _) {
          final stored = widget.model.snapshot.settings[widget.setting];
          final known = widget.choices.any((c) => c.value == stored);
          final custom = stored is String && !known;
          final value = known || custom ? stored : widget.choices.first.value;
          final own = widget.own;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Header(title: widget.title, onBack: widget.onDone),
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
                  children: [
                    if (widget.hint.isNotEmpty)
                      Padding(
                        padding: const EdgeInsets.fromLTRB(4, 0, 4, 12),
                        child: Text(widget.hint, style: Ink.note),
                      ),
                    for (final c in widget.choices) ...[
                      Tile(
                        title: c.title,
                        subtitle: c.hint,
                        selected: !custom && !_typing && value == c.value,
                        arrow: false,
                        wrap: true,
                        onTap: () => _pick(c.value),
                      ),
                      const SizedBox(height: 8),
                    ],
                    if (own != null) ...[
                      Tile(
                        title: own,
                        subtitle: custom ? stored : widget.ownHint,
                        selected: custom && !_typing,
                        arrow: !custom,
                        wrap: true,
                        onTap: () {
                          setState(() => _typing = true);
                          _focus.requestFocus();
                        },
                      ),
                      if (_typing) ...[
                        const SizedBox(height: 8),
                        Panel(
                          padding: const EdgeInsets.all(16),
                          child: Stack(
                            children: [
                              ValueListenableBuilder<TextEditingValue>(
                                valueListenable: _text,
                                builder: (context, v, _) => v.text.isEmpty
                                    ? Text(
                                        switch (widget.setting) {
                                          dnsKey => S.dnsExample,
                                          probeUrlKey => S.probeExample,
                                          _ => widget.ownHint,
                                        },
                                        style: Ink.body.copyWith(color: Ink.faint),
                                        maxLines: 1,
                                        overflow: TextOverflow.ellipsis,
                                      )
                                    : const SizedBox.shrink(),
                              ),
                              Semantics(
                                textField: true,
                                label: own,
                                child: EditableText(
                                  controller: _text,
                                  focusNode: _focus,
                                  style: Ink.body,
                                  cursorColor: Ink.accent,
                                  backgroundCursorColor: Ink.faint,
                                  selectionColor: Ink.selected,
                                  keyboardType: TextInputType.url,
                                  textInputAction: TextInputAction.done,
                                  autocorrect: false,
                                  enableSuggestions: false,
                                  inputFormatters: [LengthLimitingTextInputFormatter(256)],
                                  maxLines: 1,
                                  onSubmitted: (_) => _save(),
                                ),
                              ),
                            ],
                          ),
                        ),
                        if (_refused) ...[const SizedBox(height: 8), Notice(S.badValue)],
                        const SizedBox(height: 8),
                        PrimaryButton(label: S.save, busy: _saving, onTap: _save),
                      ],
                    ],
                  ],
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}
