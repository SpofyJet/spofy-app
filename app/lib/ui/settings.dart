import 'package:flutter/widgets.dart';

import '../state/model.dart';
import 'choice.dart';
import 'icons.dart';
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
const remindKey = 'remind';

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

List<Choice> get ipv6Choices => [Choice(null, S.ipv6Auto, S.ipv6AutoHint), Choice(false, S.ipv6Off, S.ipv6OffHint)];

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

List<Choice> get remindChoices => [
  Choice(1, S.remindBefore(1)),
  Choice(null, S.remindBefore(3)),
  Choice(5, S.remindBefore(5)),
  Choice(7, S.remindBefore(7)),
  Choice(0, S.remindOff),
];

String chosen(List<Choice> choices, Object? value) {
  for (final c in choices) {
    if (c.value == value) {
      return c.title;
    }
  }
  return value is String ? value : choices.first.title;
}

String advancedSummary(Map<String, Object?> s) {
  final parts = {
    S.fragment: s[fragmentKey] == true,
    S.dns: s[dnsKey] != null,
    S.ipv6: s[ipv6Key] == false,
    S.lan: s[lanKey] == true,
    S.checking: s[probeUrlKey] != null || (s[probeTimeoutKey] ?? 4) != 4,
  };
  final changed = [
    for (final MapEntry(:key, :value) in parts.entries)
      if (value) key,
  ];
  return changed.isEmpty ? S.advancedAll(parts.keys.take(4)) : S.advancedChanged(changed);
}

typedef ChoiceSet = ({String title, List<Choice> choices, String hint, String? own, String ownHint});

ChoiceSet choiceSet(String setting) => switch (setting) {
  dnsKey => (title: S.dns, choices: dnsChoices, hint: S.dnsHint, own: S.dnsOwn, ownHint: S.dnsOwnHint),
  ipv6Key => (title: S.ipv6, choices: ipv6Choices, hint: S.ipv6Hint, own: null, ownHint: ''),
  probeUrlKey => (
    title: S.probeTarget,
    choices: probeChoices,
    hint: S.probeHint,
    own: S.probeOwn,
    ownHint: S.probeOwnHint,
  ),
  remindKey => (title: S.remind, choices: remindChoices, hint: S.remindHint, own: null, ownHint: ''),
  themeKey => (title: S.theme, choices: themeChoices, hint: '', own: null, ownHint: ''),
  _ => (title: S.language, choices: languageChoices, hint: '', own: null, ownHint: ''),
};

Widget? mark(BuildContext context, Glyph glyph) =>
    MediaQuery.textScalerOf(context).scale(100) > 115 || MediaQuery.sizeOf(context).width < 340
    ? null
    : GlyphMark(glyph);

class ChoiceTile extends StatelessWidget {
  const ChoiceTile(this.glyph, this.setting, {super.key, required this.model, required this.open});

  final Glyph glyph;
  final String setting;
  final AppModel model;
  final void Function(Widget Function(VoidCallback done) screen) open;

  @override
  Widget build(BuildContext context) {
    final choice = choiceSet(setting);
    return Tile(
      leading: mark(context, glyph),
      title: choice.title,
      subtitle: chosen(choice.choices, model.snapshot.settings[setting]),
      onTap: () => open((done) {
        final choice = choiceSet(setting);
        return ChoiceScreen(
          model: model,
          title: choice.title,
          setting: setting,
          choices: choice.choices,
          hint: choice.hint,
          own: choice.own,
          ownHint: choice.ownHint,
          onDone: done,
        );
      }),
    );
  }
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
                      leading: mark(context, Glyph.power),
                      title: S.autoConnect,
                      value: settings[autoConnectKey] == true,
                      onChanged: (value) => model.set(autoConnectKey, value),
                    ),
                    gap,
                    SwitchTile(
                      leading: mark(context, Glyph.block),
                      title: S.killSwitch,
                      subtitle: S.killSwitchHint,
                      value: settings[killSwitchKey] == true,
                      onChanged: (value) => model.set(killSwitchKey, value),
                    ),
                    if (model.always != null) ...[
                      gap,
                      Tile(
                        leading: mark(context, Glyph.always),
                        title: S.alwaysOn,
                        subtitle: S.alwaysOnHint,
                        wrap: true,
                        onTap: model.always,
                      ),
                    ],
                    SectionTitle(S.network),
                    if (model.apps != null && onApps != null) ...[
                      Tile(
                        leading: mark(context, Glyph.apps),
                        title: S.apps,
                        subtitle: S.appsSummary(model.chosenApps),
                        onTap: onApps,
                      ),
                      gap,
                    ],
                    Tile(
                      leading: mark(context, Glyph.network),
                      title: S.advanced,
                      subtitle: advancedSummary(settings),
                      wrap: true,
                      onTap: () => open((done) => AdvancedScreen(model: model, open: open, onDone: done)),
                    ),
                    SectionTitle(S.notices),
                    ChoiceTile(Glyph.bell, remindKey, model: model, open: open),
                    SectionTitle(S.look),
                    ChoiceTile(Glyph.theme, themeKey, model: model, open: open),
                    gap,
                    ChoiceTile(Glyph.globe, languageKey, model: model, open: open),
                    SectionTitle(S.about),
                    if (onSupport != null) ...[
                      Tile(leading: mark(context, Glyph.chat), title: S.support, onTap: onSupport),
                      gap,
                    ],
                    if (onLogs != null) ...[
                      Tile(
                        leading: mark(context, Glyph.log),
                        title: S.logs,
                        subtitle: S.logsHint,
                        wrap: true,
                        onTap: onLogs,
                      ),
                      gap,
                    ],
                    SwitchTile(
                      leading: mark(context, Glyph.search),
                      title: S.logsDetailed,
                      subtitle: S.logsDetailedHint,
                      value: settings[logLevelKey] == 'detailed',
                      onChanged: (value) => model.set(logLevelKey, value ? 'detailed' : null),
                    ),
                    if (onAbout != null) ...[
                      gap,
                      Tile(
                        leading: mark(context, Glyph.info),
                        title: S.version(model.snapshot.version),
                        subtitle: model.update == null ? '' : S.updateBanner(model.update!),
                        onTap: onAbout,
                      ),
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

class AdvancedScreen extends StatelessWidget {
  const AdvancedScreen({super.key, required this.model, required this.open, required this.onDone});

  final AppModel model;
  final void Function(Widget Function(VoidCallback done) screen) open;
  final VoidCallback onDone;

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
              Header(title: S.advanced, onBack: onDone),
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.fromLTRB(Ink.gutter, 0, Ink.gutter, 16),
                  children: [
                    SectionTitle(S.tunnel),
                    SwitchTile(
                      leading: mark(context, Glyph.cut),
                      title: S.fragment,
                      subtitle: S.fragmentHint,
                      value: settings[fragmentKey] == true,
                      onChanged: (value) => model.set(fragmentKey, value ? true : null),
                    ),
                    gap,
                    ChoiceTile(Glyph.dns, dnsKey, model: model, open: open),
                    gap,
                    ChoiceTile(Glyph.route, ipv6Key, model: model, open: open),
                    gap,
                    SwitchTile(
                      leading: mark(context, Glyph.home),
                      title: S.lan,
                      subtitle: S.lanHint,
                      value: settings[lanKey] == true,
                      onChanged: (value) => model.set(lanKey, value ? true : null),
                    ),
                    SectionTitle(S.checking),
                    ChoiceTile(Glyph.pulse, probeUrlKey, model: model, open: open),
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
