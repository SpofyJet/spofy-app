import 'dart:async';
import 'dart:collection';
import 'dart:convert';
import 'dart:io';

import 'client.dart';

class ProcessTransport implements Transport {
  ProcessTransport._(this._process) : lines = _process.stdout.transform(utf8.decoder).transform(const LineSplitter()) {
    _process.stderr.transform(utf8.decoder).transform(const LineSplitter()).listen(_remember, onError: (_) {});
  }

  static const _kept = 200;

  static Future<ProcessTransport> start(
    String executable,
    List<String> arguments, {
    Map<String, String> environment = const {},
  }) async {
    return ProcessTransport._(await Process.start(executable, arguments, environment: environment));
  }

  final Process _process;
  final _log = ListQueue<String>();

  @override
  final Stream<String> lines;

  List<String> get log => List.unmodifiable(_log);

  Future<int> get exitCode => _process.exitCode;

  @override
  void send(String line) {
    _process.stdin.writeln(line);
  }

  @override
  Future<void> close() async {
    try {
      await _process.stdin.close();
    } on IOException {
      _process.kill();
    }
    await _process.exitCode.timeout(
      const Duration(seconds: 5),
      onTimeout: () {
        _process.kill(ProcessSignal.sigkill);
        return _process.exitCode;
      },
    );
  }

  void _remember(String line) {
    _log.addLast(line);
    if (_log.length > _kept) {
      _log.removeFirst();
    }
  }
}
