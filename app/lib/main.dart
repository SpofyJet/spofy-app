import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import 'core/client.dart';
import 'host/host.dart';
import 'state/model.dart';
import 'ui/app.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  if (Platform.isAndroid) {
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
  }
  final platform = host();
  final model = AppModel.opening(
    () async => CoreClient(await platform.open()),
    permit: platform.permit,
    always: platform.always,
    apps: platform.apps,
    proxy: platform.proxy,
    browse: platform.browse,
    install: platform.install,
  );
  platform.links(model.offer);
  runApp(SpofyApp(model: model, dress: platform.dress, hold: Platform.isWindows));
}
