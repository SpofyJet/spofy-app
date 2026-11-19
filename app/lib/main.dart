import 'dart:async';

import 'package:flutter/widgets.dart';

import 'core/client.dart';
import 'host/host.dart';
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

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final platform = host();
  Future<CoreClient> open() async => CoreClient(await platform.open());
  CoreClient first;
  try {
    first = await open();
  } on Object {
    first = CoreClient(_NoCore());
  }
  final model = AppModel(first, reopen: open, permit: platform.permit);
  unawaited(model.start());
  runApp(SpofyApp(model: model));
}
