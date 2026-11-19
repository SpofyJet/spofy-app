import 'package:flutter/widgets.dart';

import '../state/model.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

const autoConnectKey = 'autoConnect';
const killSwitchKey = 'killSwitch';

class SettingsScreen extends StatelessWidget {
  const SettingsScreen({super.key, required this.model, required this.onDone});

  final AppModel model;
  final VoidCallback onDone;

  @override
  Widget build(BuildContext context) {
    return Backdrop(
      child: ListenableBuilder(
        listenable: model,
        builder: (context, _) {
          final settings = model.snapshot.settings;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Header(title: S.settings, onBack: onDone),
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
                  children: [
                    SwitchTile(
                      title: S.autoConnect,
                      subtitle: S.autoConnectHint,
                      value: settings[autoConnectKey] == true,
                      onChanged: (value) => model.set(autoConnectKey, value),
                    ),
                    const SizedBox(height: 8),
                    SwitchTile(
                      title: S.killSwitch,
                      subtitle: S.killSwitchHint,
                      value: settings[killSwitchKey] == true,
                      onChanged: (value) => model.set(killSwitchKey, value),
                    ),
                    if (model.always != null) ...[
                      const SizedBox(height: 8),
                      Tile(title: S.alwaysOn, subtitle: S.alwaysOnHint, wrap: true, onTap: model.always),
                    ],
                    const SizedBox(height: 20),
                    Text(S.version(model.snapshot.version), style: Ink.label, textAlign: TextAlign.center),
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
