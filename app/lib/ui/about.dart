import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/model.dart';
import 'icons.dart';
import 'strings.dart';
import 'theme.dart';
import 'update.dart';
import 'widgets.dart';

const sourceAddress = 'https://github.com/SpofyJet/spofy-app';

const coreLicenses = [
  (
    'Xray-core',
    'Mozilla Public License 2.0\nhttps://mozilla.org/MPL/2.0/\n\nИсходный код ядра в Spofy: https://github.com/SpofyJet/xray-core',
  ),
  ('Wintun', 'Prebuilt Binaries License, WireGuard LLC\nhttps://www.wintun.net'),
  ('wireguard-go', 'MIT License, WireGuard LLC'),
  ('Go', 'BSD 3-Clause License, The Go Authors\nhttps://go.dev/LICENSE'),
  ('Модули ядра', 'Список и версии: $sourceAddress/blob/main/core/go.mod'),
];

class AboutScreen extends StatefulWidget {
  const AboutScreen({super.key, required this.model, required this.onDone, required this.open});

  final AppModel model;
  final VoidCallback onDone;
  final void Function(Widget Function(VoidCallback done) screen) open;

  @override
  State<AboutScreen> createState() => _AboutScreenState();
}

class _AboutScreenState extends State<AboutScreen> {
  Report? _report;
  bool _checked = false;

  @override
  void initState() {
    super.initState();
    unawaited(_load());
  }

  Future<void> _load() async {
    final report = await widget.model.report();
    if (mounted) {
      setState(() => _report = report);
    }
  }

  Future<void> _check() async {
    await widget.model.checkUpdate();
    if (mounted) {
      setState(() => _checked = true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final model = widget.model;
    return Backdrop(
      child: ListenableBuilder(
        listenable: model,
        builder: (context, _) {
          final snapshot = model.snapshot;
          final update = model.update;
          final report = _report;
          final failure = model.failure;
          final checking = model.running(Task.checkUpdate);
          final browse = model.browse;
          final device = report == null
              ? ''
              : [report.model, '${report.os} ${report.osVersion}'.trim()].where((p) => p.isNotEmpty).join(' · ');
          const gap = SizedBox(height: 8);
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Header(title: S.about, onBack: widget.onDone),
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
                  children: [
                    Panel(
                      padding: const EdgeInsets.fromLTRB(20, 22, 20, 20),
                      child: Column(
                        children: [
                          Text(S.app, style: Ink.wordmark),
                          const SizedBox(height: 6),
                          Text(S.release(snapshot.version, snapshot.build), style: Ink.note),
                        ],
                      ),
                    ),
                    gap,
                    if (update != null)
                      UpdatePanel(model: model, update: update)
                    else ...[
                      if (_checked && !checking && failure?.task == Task.checkUpdate)
                        Padding(
                          padding: const EdgeInsets.only(bottom: 8),
                          child: failure!.code == 'update_none'
                              ? const Notice(S.updateLatest, color: Ink.good, glyph: Glyph.check)
                              : Notice(S.failure(failure)),
                        ),
                      QuietButton(label: S.updateCheck, busy: checking, onTap: checking ? null : _check),
                    ],
                    const SizedBox(height: 20),
                    Tile(title: S.engine, subtitle: snapshot.engine.isEmpty ? '—' : snapshot.engine, arrow: false),
                    gap,
                    Tile(title: S.device, subtitle: device.isEmpty ? '—' : device, arrow: false),
                    gap,
                    Tile(
                      title: S.sourceCode,
                      subtitle: 'github.com/SpofyJet/spofy-app',
                      onTap: browse == null ? null : () => browse(sourceAddress),
                    ),
                    gap,
                    Tile(
                      title: S.licenses,
                      subtitle: S.licensesHint,
                      wrap: true,
                      onTap: () => widget.open((done) => LicensesScreen(onDone: done)),
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

class LicensesScreen extends StatefulWidget {
  const LicensesScreen({super.key, required this.onDone});

  final VoidCallback onDone;

  @override
  State<LicensesScreen> createState() => _LicensesScreenState();
}

class _LicensesScreenState extends State<LicensesScreen> {
  final Map<String, List<String>> _found = {
    for (final (name, text) in coreLicenses) name: [text],
  };
  StreamSubscription<LicenseEntry>? _reading;
  bool _done = false;
  String? _open;

  @override
  void initState() {
    super.initState();
    _reading = LicenseRegistry.licenses.listen(
      (entry) {
        final text = entry.paragraphs.map((p) => '${'  ' * p.indent.clamp(0, 8)}${p.text}').join('\n\n');
        for (final name in entry.packages) {
          _found.putIfAbsent(name, () => []).add(text);
        }
      },
      onDone: () {
        if (mounted) {
          setState(() => _done = true);
        }
      },
    );
  }

  @override
  void dispose() {
    _reading?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final names = _found.keys.toList()..sort((a, b) => a.toLowerCase().compareTo(b.toLowerCase()));
    final open = _open;
    if (open != null) {
      return Backdrop(
        child: PopScope(
          canPop: false,
          onPopInvokedWithResult: (didPop, _) => setState(() => _open = null),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Header(title: open, onBack: () => setState(() => _open = null)),
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
                  children: [
                    for (final text in _found[open] ?? const <String>[]) ...[
                      Panel(child: Text(text, style: Ink.type(12.5, 400, Ink.muted, height: 1.45))),
                      const SizedBox(height: 8),
                    ],
                  ],
                ),
              ),
            ],
          ),
        ),
      );
    }
    return Backdrop(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Header(title: S.licenses, onBack: widget.onDone),
          Expanded(
            child: !_done
                ? const Center(child: Spinner(size: 28, color: Ink.accent))
                : ListView.separated(
                    padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
                    itemCount: names.length,
                    separatorBuilder: (_, _) => const SizedBox(height: 8),
                    itemBuilder: (context, i) => Tile(
                      title: names[i],
                      subtitle: S.licenseCount(_found[names[i]]!.length),
                      onTap: () => setState(() => _open = names[i]),
                    ),
                  ),
          ),
        ],
      ),
    );
  }
}
