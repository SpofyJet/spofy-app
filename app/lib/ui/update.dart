import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/model.dart';
import 'icons.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

const updateShownKey = 'updateShown';

bool shouldOffer(AppModel model) {
  final update = model.update;
  if (update == null) {
    return false;
  }
  return update.required || model.snapshot.settings[updateShownKey] != update.build;
}

class UpdatePanel extends StatelessWidget {
  const UpdatePanel({super.key, required this.model, required this.update});

  final AppModel model;
  final Update update;

  Future<void> _go() async {
    if (!update.ready && !await model.fetchUpdate()) {
      return;
    }
    await model.installUpdate();
  }

  @override
  Widget build(BuildContext context) {
    final fetching = model.running(Task.fetchUpdate);
    final installing = model.running(Task.installUpdate);
    final failure = model.failure;
    final failed = failure != null && (failure.task == Task.fetchUpdate || failure.task == Task.installUpdate);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Panel(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(S.updateTitle(update), style: Ink.heading),
              if (update.notes.isNotEmpty) ...[
                const SizedBox(height: 10),
                Text(update.notes, style: Ink.note.copyWith(color: Ink.text)),
              ],
              const SizedBox(height: 14),
              if (fetching || update.done > 0) ...[
                Meter(update.ready ? 1 : update.share, color: update.ready ? Ink.good : Ink.accent),
                const SizedBox(height: 8),
              ],
              Text(S.updateProgress(update), style: Ink.label),
              const SizedBox(height: 4),
              Text(S.updateSigned, style: Ink.type(12, 400, Ink.faint)),
            ],
          ),
        ),
        if (update.required) ...[const SizedBox(height: 8), const Notice(S.updateRequired)],
        if (failed) ...[const SizedBox(height: 8), Notice(S.failure(failure))],
        if (model.connection.state.protecting) ...[
          const SizedBox(height: 8),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 4),
            child: Text(S.updateVpn, style: Ink.note),
          ),
        ],
        const SizedBox(height: 12),
        PrimaryButton(label: update.ready ? S.updateInstall : S.updateNow, busy: fetching || installing, onTap: _go),
      ],
    );
  }
}

class UpdateScreen extends StatelessWidget {
  const UpdateScreen({super.key, required this.model, required this.onDone});

  final AppModel model;
  final VoidCallback onDone;

  @override
  Widget build(BuildContext context) {
    return Backdrop(
      child: ListenableBuilder(
        listenable: model,
        builder: (context, _) {
          final update = model.update;
          final required = update?.required ?? false;
          return PopScope(
            canPop: !required,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Header(title: S.update, onBack: required ? null : onDone),
                Expanded(
                  child: ListView(
                    padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
                    children: [
                      if (update == null)
                        const Notice(S.updateLatest, color: Ink.good, glyph: Glyph.check)
                      else
                        UpdatePanel(model: model, update: update),
                      if (!required) ...[
                        const SizedBox(height: 8),
                        QuietButton(label: S.updateLater, color: Ink.muted, onTap: onDone),
                      ],
                    ],
                  ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}
