import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/apps.dart';
import '../state/model.dart';
import 'settings.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

String _two(int n) => n.toString().padLeft(2, '0');

String _stamp(int seconds) {
  final t = DateTime.fromMillisecondsSinceEpoch(seconds * 1000, isUtc: true);
  return '${_two(t.day)}.${_two(t.month)} ${_two(t.hour)}:${_two(t.minute)}:${_two(t.second)}';
}

String supportReport(Snapshot snapshot, Report? report, DateTime now) {
  final c = snapshot.connection;
  final s = snapshot.subscription;
  final names = {for (final server in snapshot.servers) server.id: S.serverName(server)};
  final chosen = AppsChoice.of(snapshot.settings[appsKey]);
  final lines = [
    '${S.app} ${snapshot.version}',
    if (report != null) '${report.os} ${report.osVersion} · ${report.model}',
    if (report != null) '${S.reportDevice}: ${report.device}',
    if (s == null)
      S.noSubscriptionTitle
    else
      '${S.subscription}: ${S.expiry(s, now)} · ${S.usage(s)} · ${S.updated} ${_stamp(s.fetchedAt)}',
    [
      c.choice == autoChoice ? S.auto : '${S.pinned}: ${names[c.choice] ?? c.choice}',
      '${S.killSwitch}: ${S.yes(snapshot.settings[killSwitchKey] == true)}',
      '${S.autoConnect}: ${S.yes(snapshot.settings[autoConnectKey] == true)}',
      '${S.apps}: ${S.appsSummary(chosen)}',
    ].join(' · '),
    [
      S.title(c.state),
      if (c.server.isNotEmpty) names[c.server] ?? c.server,
      if (c.delayMs > 0) S.delay(c.delayMs),
      if (c.code.isNotEmpty) c.code,
    ].join(' · '),
    if (snapshot.servers.isNotEmpty) '${S.servers}:',
    for (final server in snapshot.servers) '${S.serverName(server)}: ${S.delay(server.delayMs)}',
    if (report != null && report.journal.isNotEmpty) '${S.reportJournal}:',
    if (report != null)
      for (final e in report.journal)
        [
          _stamp(e.at),
          e.state.name,
          if (e.server.isNotEmpty) names[e.server] ?? e.server,
          if (e.code.isNotEmpty) e.code,
        ].join(' '),
  ];
  return lines.join('\n');
}

class SupportScreen extends StatefulWidget {
  const SupportScreen({super.key, required this.model, required this.onDone, required this.now});

  final AppModel model;
  final VoidCallback onDone;
  final DateTime Function() now;

  @override
  State<SupportScreen> createState() => _SupportScreenState();
}

class _SupportScreenState extends State<SupportScreen> {
  Report? _report;
  bool _copied = false;
  Timer? _later;

  @override
  void initState() {
    super.initState();
    unawaited(_load());
  }

  Future<void> _load() async {
    final report = await widget.model.report();
    if (mounted && report != null) {
      setState(() => _report = report);
    }
  }

  @override
  void dispose() {
    _later?.cancel();
    super.dispose();
  }

  Future<void> _copy(String text) async {
    await Clipboard.setData(ClipboardData(text: text));
    if (!mounted) {
      return;
    }
    setState(() => _copied = true);
    _later?.cancel();
    _later = Timer(const Duration(seconds: 2), () => setState(() => _copied = false));
  }

  @override
  Widget build(BuildContext context) {
    return Backdrop(
      child: ListenableBuilder(
        listenable: widget.model,
        builder: (context, _) {
          final text = supportReport(widget.model.snapshot, _report, widget.now());
          final address = Uri.tryParse(widget.model.subscription?.supportUrl ?? '');
          final browse = widget.model.browse;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Header(title: S.support, onBack: widget.onDone),
              Padding(
                padding: const EdgeInsets.fromLTRB(Ink.gutter + 4, 4, Ink.gutter + 4, 12),
                child: Text(S.supportHint, style: Ink.note),
              ),
              Expanded(
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: Ink.gutter),
                  child: Panel(
                    padding: EdgeInsets.zero,
                    child: SingleChildScrollView(
                      padding: const EdgeInsets.all(16),
                      child: Text(text, style: Ink.label.copyWith(height: 1.45)),
                    ),
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(Ink.gutter, 12, Ink.gutter, 8),
                child: PrimaryButton(label: _copied ? S.copied : S.copyReport, onTap: () => _copy(text)),
              ),
              if (browse != null && address != null && address.isScheme('https'))
                Padding(
                  padding: const EdgeInsets.fromLTRB(Ink.gutter, 0, Ink.gutter, 8),
                  child: QuietButton(label: S.writeSupport, onTap: () => browse(address.toString())),
                ),
              const SizedBox(height: 8),
            ],
          );
        },
      ),
    );
  }
}
