import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/model.dart';
import 'icons.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

String _two(int n) => n.toString().padLeft(2, '0');

String _clock(int at) {
  final t = DateTime.fromMillisecondsSinceEpoch(at * 1000);
  return '${_two(t.hour)}:${_two(t.minute)}:${_two(t.second)}';
}

String logText(List<LogLine> lines) => [
  for (final l in lines)
    () {
      final t = DateTime.fromMillisecondsSinceEpoch(l.at * 1000);
      final repeat = l.count > 1 ? '  ×${l.count} … ${_clock(l.last)}' : '';
      return '${_two(t.day)}.${_two(t.month)} ${_clock(l.at)}  ${l.source}  ${l.text}$repeat';
    }(),
].join('\n');

class LogsScreen extends StatefulWidget {
  const LogsScreen({super.key, required this.model, required this.onDone});

  final AppModel model;
  final VoidCallback onDone;

  @override
  State<LogsScreen> createState() => _LogsScreenState();
}

class _LogsScreenState extends State<LogsScreen> {
  List<LogLine>? _lines;
  final _open = <int>{};
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
    setState(() {
      _lines = lines ?? const [];
      _open.clear();
    });
    WidgetsBinding.instance.addPostFrameCallback((_) => _end());
  }

  void _end() {
    if (!mounted || !_scroll.hasClients || _scroll.position.pixels >= _scroll.position.maxScrollExtent) {
      return;
    }
    _scroll.jumpTo(_scroll.position.maxScrollExtent);
    WidgetsBinding.instance.addPostFrameCallback((_) => _end());
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
                    ? Center(child: Spinner(size: 28, color: Ink.accent))
                    : lines.isEmpty
                    ? Center(child: Text(S.logsEmpty, style: Ink.note))
                    : ListView.builder(
                        controller: _scroll,
                        padding: const EdgeInsets.all(14),
                        itemCount: lines.length,
                        itemBuilder: (context, i) => _Line(
                          lines[i],
                          open: _open.contains(i),
                          onTap: () => setState(() => _open.contains(i) ? _open.remove(i) : _open.add(i)),
                        ),
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
  const _Line(this.line, {required this.open, required this.onTap});

  final LogLine line;
  final bool open;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final cause = S.logsCause(line.cause);
    final repeat = line.count > 1 ? S.logsRepeat(line.count, _clock(line.last)) : null;
    final small = Ink.type(12, 400, Ink.muted, height: 1.4);
    final body = Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text.rich(
          TextSpan(
            children: [
              TextSpan(
                text: '${_clock(line.at)}  ',
                style: TextStyle(color: Ink.faint),
              ),
              TextSpan(
                text: cause ?? line.text,
                style: TextStyle(color: cause == null && line.source == 'engine' ? Ink.muted : Ink.text),
              ),
            ],
          ),
          style: Ink.type(12.5, 400, Ink.text, height: 1.4),
        ),
        if (repeat != null) Text(repeat, style: small),
        if (cause != null) Text(open ? line.text : S.logsRaw, style: small),
      ],
    );
    if (cause == null) {
      return Padding(padding: const EdgeInsets.only(bottom: 8), child: body);
    }
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Pressable(
        label: [cause, ?repeat, open ? line.text : S.logsRaw].join('. '),
        onTap: onTap,
        radius: const BorderRadius.all(Radius.circular(8)),
        child: ConstrainedBox(
          constraints: const BoxConstraints(minHeight: 48),
          child: body,
        ),
      ),
    );
  }
}
