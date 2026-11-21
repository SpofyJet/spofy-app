import 'package:flutter/widgets.dart';

import '../state/model.dart';
import 'choice.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

const autoConnectKey = 'autoConnect';
const killSwitchKey = 'killSwitch';
const probeUrlKey = 'probeUrl';
const probeTimeoutKey = 'probeTimeout';
const fragmentKey = 'fragment';
const dnsKey = 'dns';
const ipv6Key = 'ipv6';
const lanKey = 'lan';
const logLevelKey = 'logLevel';

const probeGoogle = 'http://www.gstatic.com/generate_204';
const probeCloudflare = 'http://cp.cloudflare.com/generate_204';

List<Choice> get probeChoices => [
  Choice(null, S.probeAuto, S.probeAutoHint),
  const Choice(probeGoogle, 'Google', 'www.gstatic.com'),
  const Choice(probeCloudflare, 'Cloudflare', 'cp.cloudflare.com'),
];

List<Choice> get dnsChoices => [
  Choice(null, S.dnsDefault, S.dnsDefaultHint),
  const Choice('cloudflare', 'Cloudflare', '1.1.1.1'),
  const Choice('google', 'Google', '8.8.8.8'),
  Choice('quad9', 'Quad9', '9.9.9.9 · ${S.dnsSafe}'),
  Choice('adguard', 'AdGuard', '94.140.14.14 · ${S.dnsClean}'),
];

List<Choice> get themeChoices {
  final system = WidgetsBinding.instance.platformDispatcher.platformBrightness;
  return [
    Choice(null, S.themeSystem, S.themeNow(paletteFor(null, system).dark)),
    Choice('light', S.themeLight),
    Choice('dark', S.themeDark),
  ];
}

List<Choice> get languageChoices => [
  Choice(null, S.languageSystem, S.languageNow(langFor(null, WidgetsBinding.instance.platformDispatcher.locales))),
  const Choice('ru', S.russian),
  const Choice('en', S.english),
];

String chosen(List<Choice> choices, Object? value) {
  for (final c in choices) {
    if (c.value == value) {
      return c.title;
    }
  }
  return value is String ? value : choices.first.title;
}

class SettingsScreen extends StatelessWidget {
  const SettingsScreen({
    super.key,
    required this.model,
    required this.onDone,
    required this.open,
    this.onApps,
    this.onSupport,
    this.onLogs,
    this.onAbout,
  });

  final AppModel model;
  final VoidCallback onDone;
  final void Function(Widget Function(VoidCallback done) screen) open;
  final VoidCallback? onApps;
  final VoidCallback? onSupport;
  final VoidCallback? onLogs;
  final VoidCallback? onAbout;

  @override
  Widget build(BuildContext context) {
    return Backdrop(
      child: ListenableBuilder(
        listenable: model,
        builder: (context, _) {
          final settings = model.snapshot.settings;
          final wait = settings[probeTimeoutKey] is int ? settings[probeTimeoutKey]! as int : 4;
          const gap = SizedBox(height: 8);
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Header(title: S.settings, onBack: onDone),
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.fromLTRB(Ink.gutter, 0, Ink.gutter, 16),
                  children: [
                    SectionTitle(S.connection),
                    SwitchTile(
                      title: S.autoConnect,
                      subtitle: S.autoConnectHint,
                      value: settings[autoConnectKey] == true,
                      onChanged: (value) => model.set(autoConnectKey, value),
                    ),
                    gap,
                    SwitchTile(
                      title: S.killSwitch,
                      subtitle: S.killSwitchHint,
                      value: settings[killSwitchKey] == true,
                      onChanged: (value) => model.set(killSwitchKey, value),
                    ),
                    if (model.apps != null && onApps != null) ...[
                      gap,
                      Tile(title: S.apps, subtitle: S.appsSummary(model.chosenApps), onTap: onApps),
                    ],
                    if (model.always != null) ...[
                      gap,
                      Tile(title: S.alwaysOn, subtitle: S.alwaysOnHint, wrap: true, onTap: model.always),
                    ],
                    SectionTitle(S.tunnel),
                    SwitchTile(
                      title: S.fragment,
                      subtitle: S.fragmentHint,
                      value: settings[fragmentKey] == true,
                      onChanged: (value) => model.set(fragmentKey, value ? true : null),
                    ),
                    gap,
                    Tile(
                      title: S.dns,
                      subtitle: chosen(dnsChoices, settings[dnsKey]),
                      onTap: () => open(
                        (done) => ChoiceScreen(
                          model: model,
                          title: S.dns,
                          setting: dnsKey,
                          choices: dnsChoices,
                          own: S.dnsOwn,
                          ownHint: S.dnsOwnHint,
                          onDone: done,
                        ),
                      ),
                    ),
                    gap,
                    SwitchTile(
                      title: S.ipv6,
                      subtitle: S.ipv6Hint,
                      value: settings[ipv6Key] != false,
                      onChanged: (value) => model.set(ipv6Key, value ? null : false),
                    ),
                    gap,
                    SwitchTile(
                      title: S.lan,
                      subtitle: S.lanHint,
                      value: settings[lanKey] == true,
                      onChanged: (value) => model.set(lanKey, value ? true : null),
                    ),
                    SectionTitle(S.checking),
                    Tile(
                      title: S.probeTarget,
                      subtitle: chosen(probeChoices, settings[probeUrlKey]),
                      onTap: () => open(
                        (done) => ChoiceScreen(
                          model: model,
                          title: S.probeTarget,
                          setting: probeUrlKey,
                          choices: probeChoices,
                          own: S.probeOwn,
                          ownHint: S.probeOwnHint,
                          onDone: done,
                        ),
                      ),
                    ),
                    gap,
                    Padding(
                      padding: const EdgeInsets.fromLTRB(4, 4, 4, 8),
                      child: Text(S.probeWaitHint, style: Ink.note),
                    ),
                    Segments<int>(
                      options: [
                        for (final n in const [2, 3, 4, 6, 10]) (n, S.seconds(n)),
                      ],
                      value: wait,
                      onChanged: (value) => model.set(probeTimeoutKey, value == 4 ? null : value),
                    ),
                    SectionTitle(S.look),
                    Tile(
                      title: S.theme,
                      subtitle: chosen(themeChoices, settings[themeKey]),
                      onTap: () => open(
                        (done) => ChoiceScreen(
                          model: model,
                          title: S.theme,
                          setting: themeKey,
                          choices: themeChoices,
                          onDone: done,
                        ),
                      ),
                    ),
                    gap,
                    Tile(
                      title: S.language,
                      subtitle: chosen(languageChoices, settings[languageKey]),
                      onTap: () => open(
                        (done) => ChoiceScreen(
                          model: model,
                          title: S.language,
                          setting: languageKey,
                          choices: languageChoices,
                          onDone: done,
                        ),
                      ),
                    ),
                    SectionTitle(S.logs),
                    SwitchTile(
                      title: S.logsDetailed,
                      subtitle: S.logsDetailedHint,
                      value: settings[logLevelKey] == 'detailed',
                      onChanged: (value) => model.set(logLevelKey, value ? 'detailed' : null),
                    ),
                    if (onLogs != null) ...[gap, Tile(title: S.logs, subtitle: S.logsHint, wrap: true, onTap: onLogs)],
                    SectionTitle(S.about),
                    if (onSupport != null) ...[Tile(title: S.support, onTap: onSupport), gap],
                    if (onAbout != null)
                      Tile(
                        title: S.about,
                        subtitle: model.update == null
                            ? S.version(model.snapshot.version)
                            : S.updateBanner(model.update!),
                        trailingColor: Ink.accent,
                        onTap: onAbout,
                      ),
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
