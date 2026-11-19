import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:flutter/widgets.dart';

import 'core/client.dart';
import 'core/process_transport.dart';
import 'state/model.dart';
import 'ui/app.dart';

class _NoCore implements Transport {
  @override
  Stream<String> get lines => const Stream.empty();

  @override
  void send(String line) {}

  @override
  Future<void> close() async {}
}

String _home() => Platform.environment['HOME'] ?? Directory.systemTemp.path;

String _key() {
  final given = Platform.environment['SPOFY_STATE_KEY'];
  if (given != null && given.isNotEmpty) {
    return given;
  }
  final file = File('${Platform.environment['XDG_CONFIG_HOME'] ?? '${_home()}/.config'}/spofy/key');
  if (file.existsSync()) {
    return file.readAsStringSync().trim();
  }
  final random = Random.secure();
  final key = base64Encode(List<int>.generate(32, (_) => random.nextInt(256)));
  file.parent.createSync(recursive: true);
  file.writeAsStringSync(key, flush: true);
  if (!Platform.isWindows) {
    Process.runSync('chmod', ['600', file.path]);
  }
  return key;
}

Future<Transport> _core() async {
  final core =
      Platform.environment['SPOFY_CORE'] ??
      '${File(Platform.resolvedExecutable).parent.path}/spofy-core${Platform.isWindows ? '.exe' : ''}';
  final state =
      Platform.environment['SPOFY_STATE'] ??
      '${Platform.environment['XDG_DATA_HOME'] ?? '${_home()}/.local/share'}/spofy';
  final mode = Platform.environment['SPOFY_MODE'] ?? 'tun';
  try {
    return await ProcessTransport.start(
      core,
      ['serve', '-state', state, '-mode', mode],
      environment: {'SPOFY_STATE_KEY': _key()},
    );
  } on IOException {
    return _NoCore();
  }
}

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final model = AppModel(CoreClient(await _core()));
  unawaited(model.start());
  runApp(SpofyApp(model: model));
}
