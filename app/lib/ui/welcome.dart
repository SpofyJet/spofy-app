import 'dart:math' as math;

import 'package:flutter/widgets.dart';

import '../state/model.dart';
import 'home.dart';
import 'icons.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

class Welcome extends StatefulWidget {
  const Welcome({super.key, required this.model, required this.onManual});

  static const column = 400.0;

  final AppModel model;
  final VoidCallback onManual;

  @override
  State<Welcome> createState() => _WelcomeState();
}

class _WelcomeState extends State<Welcome> {
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
    final model = widget.model;
    final failure = model.failure;
    final problem = _nothing ? S.noLink : (failure?.task == Task.subscribe ? S.failure(failure!) : '');
    final slot = (MediaQuery.textScalerOf(context).scale(Ink.note.fontSize!) * Ink.note.height! / 2).ceil() * 4.0;
    return LayoutBuilder(
      builder: (context, box) {
        final side = math.max(Ink.gutter, (box.maxWidth - Welcome.column) / 2);
        return SingleChildScrollView(
          child: ConstrainedBox(
            constraints: BoxConstraints(minHeight: box.maxHeight),
            child: IntrinsicHeight(
              child: Padding(
                padding: EdgeInsets.fromLTRB(side, 0, side, 16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const Spacer(flex: 2),
                    Semantics(
                      header: true,
                      child: Text(S.noSubscriptionTitle, style: Ink.title, textAlign: TextAlign.center),
                    ),
                    const SizedBox(height: 8),
                    ConstrainedBox(
                      constraints: BoxConstraints(minHeight: slot),
                      child: Semantics(
                        liveRegion: true,
                        child: problem.isEmpty
                            ? Text(S.copyLink, style: Ink.note, textAlign: TextAlign.center)
                            : _Problem(problem),
                      ),
                    ),
                    if (model.browse case final browse?) ...[
                      const SizedBox(height: 8),
                      _Ask(
                        ask: problem.isEmpty ? S.buyAsk : S.stuckAsk,
                        action: problem.isEmpty ? S.buy : S.stuck,
                        onTap: () => browse(shopAddress),
                      ),
                    ],
                    const Spacer(flex: 3),
                    QuietButton(label: S.manual, onTap: widget.onManual),
                    const SizedBox(height: 8),
                    PrimaryButton(label: S.pasteLink, busy: model.running(Task.subscribe), onTap: _paste),
                  ],
                ),
              ),
            ),
          ),
        );
      },
    );
  }
}

class _Problem extends StatelessWidget {
  const _Problem(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    return Text.rich(
      TextSpan(
        children: [
          WidgetSpan(
            alignment: PlaceholderAlignment.middle,
            child: Padding(
              padding: const EdgeInsets.only(right: 6),
              child: GlyphIcon(Glyph.warning, color: Ink.warning, size: 16),
            ),
          ),
          TextSpan(text: text),
        ],
      ),
      style: Ink.note.copyWith(color: Ink.text),
      textAlign: TextAlign.center,
    );
  }
}

class _Ask extends StatelessWidget {
  const _Ask({required this.ask, required this.action, required this.onTap});

  final String ask;
  final String action;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Pressable(
      label: '$ask $action',
      onTap: onTap,
      child: ConstrainedBox(
        constraints: const BoxConstraints(minHeight: 48),
        child: Center(
          child: Text.rich(
            TextSpan(
              text: '$ask ',
              children: [
                TextSpan(
                  text: action.replaceAll(' ', '\u00A0'),
                  style: TextStyle(color: Ink.accent, fontWeight: FontWeight.w600),
                ),
              ],
            ),
            style: Ink.note,
            textAlign: TextAlign.center,
          ),
        ),
      ),
    );
  }
}
