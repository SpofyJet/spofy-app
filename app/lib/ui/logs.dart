import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/model.dart';
import 'icons.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

String logText(List<LogLine> lines) {
  String two(int n) => n.toString().padLeft(2, '0');
  return [
    for (final l in lines)
      () {
        final t = DateTime.fromMillisecondsSinceEpoch(l.at * 1000);
        return '${two(t.day)}.${two(t.month)} ${two(t.hour)}:${two(t.minute)}:${two(t.second)}  ${l.source}  ${l.text}';
      }(),
  ].join('\n');
}

class LogsScreen extends StatefulWidget {
  const LogsScreen({super.key, required this.model, required this.onDone});

  final AppModel model;
  final VoidCallback onDone;

  @override
  State<LogsScreen> createState() => _LogsScreenState();
}

class _LogsScreenState extends State<LogsScreen> {
  List<LogLine>? _lines;
  bool _copied = false;
  Timer? _later;
  final _scroll = ScrollController();

  @override
  void initState() {
    super.initState();
    unawaited(_load());
  }

  @override
  void dispose() {
    _later?.cancel();
    _scroll.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    final lines = await widget.model.logs();
    if (!mounted) {
      return;
    }
    setState(() => _lines = lines ?? const []);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scroll.hasClients) {
        _scroll.jumpTo(_scroll.position.maxScrollExtent);
      }
    });
  }

  Future<void> _copy() async {
    await Clipboard.setData(
      ClipboardData(text: '${S.app} ${widget.model.snapshot.version}\n${logText(_lines ?? const [])}'),
    );
    if (!mounted) {
      return;
    }
    setState(() => _copied = true);
    _later?.cancel();
    _later = Timer(const Duration(seconds: 2), () => setState(() => _copied = false));
  }

  Future<void> _clear() async {
    await widget.model.clearLogs();
    await _load();
  }

  @override
  Widget build(BuildContext context) {
    final lines = _lines;
    return Backdrop(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Header(
            title: S.logs,
            onBack: widget.onDone,
            action: RoundButton(glyph: Glyph.refresh, label: S.refresh, onTap: _load),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(Ink.gutter + 4, 0, Ink.gutter + 4, 12),
            child: Text(S.logsHint, style: Ink.note),
          ),
          Expanded(
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: Ink.gutter),
              child: Panel(
                padding: EdgeInsets.zero,
                child: lines == null
                    ? const Center(child: Spinner(size: 28, color: Ink.accent))
                    : lines.isEmpty
                    ? Center(child: Text(S.logsEmpty, style: Ink.note))
                    : ListView.builder(
                        controller: _scroll,
                        padding: const EdgeInsets.all(14),
                        itemCount: lines.length,
                        itemBuilder: (context, i) => _Line(lines[i]),
                      ),
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(Ink.gutter, 12, Ink.gutter, 8),
            child: Row(
              children: [
                Expanded(
                  child: QuietButton(label: S.clear, color: Ink.muted, onTap: lines == null ? null : _clear),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: PrimaryButton(
                    label: _copied ? S.copied : S.copy,
                    onTap: lines == null || lines.isEmpty ? null : _copy,
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 8),
        ],
      ),
    );
  }
}

class _Line extends StatelessWidget {
  const _Line(this.line);

  final LogLine line;

  @override
  Widget build(BuildContext context) {
    final t = DateTime.fromMillisecondsSinceEpoch(line.at * 1000);
    String two(int n) => n.toString().padLeft(2, '0');
    final engine = line.source == 'engine';
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Text.rich(
        TextSpan(
          children: [
            TextSpan(
              text: '${two(t.hour)}:${two(t.minute)}:${two(t.second)}  ',
              style: TextStyle(color: Ink.faint),
            ),
            TextSpan(
              text: line.text,
              style: TextStyle(color: engine ? Ink.muted : Ink.text),
            ),
          ],
        ),
        style: Ink.type(12.5, 400, Ink.text, height: 1.4),
      ),
    );
  }
}
