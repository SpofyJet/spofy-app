import 'dart:async';
import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/client.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/state/model.dart';
import 'package:spofy/ui/app.dart';

class SilentTransport implements Transport {
  final sent = <String>[];
  final _never = StreamController<String>();

  @override
  Stream<String> get lines => _never.stream;

  @override
  void send(String line) => sent.add(line);

  @override
  Future<void> close() => _never.close();
}

final clock = DateTime.utc(2026, 11, 19, 12);

const _names = [
  ('nl', 'Нидерланды', 'NL', 38, 0),
  ('de', 'Германия', 'DE', 44, 0),
  ('fi', 'Финляндия', 'FI', 51, 0),
  ('pl', 'Польша', 'PL', 57, 0),
  ('lt', 'Литва', 'LT', 63, 0),
  ('ie', 'Ирландия', 'IE', 74, 0),
  ('by', 'Беларусь', 'BY', 81, 0),
  ('us', 'США', 'US', 142, 0),
  ('sg', 'Сингапур', 'SG', 0, 0),
  ('bypass1', 'Обход 1', 'RU', 29, 0),
  ('bypass2', 'Обход 2', 'RU', 0, 0),
];

List<Server> servers({bool measured = true}) => [
  for (final (id, name, country, delay, tier) in _names)
    Server(
      id: id,
      name: name,
      label: name,
      country: country,
      protocol: 'vless',
      tier: tier,
      delayMs: measured ? delay : 0,
      measuredAt: measured && delay > 0 ? clock.millisecondsSinceEpoch ~/ 1000 - 600 : 0,
    ),
];

Subscription subscription({int days = 42, int used = 13312, int total = 102400}) => Subscription(
  title: 'Spofy VPN',
  upload: 1024 * 1024 * 1024,
  download: used * 1024 * 1024 - 1024 * 1024 * 1024,
  total: total * 1024 * 1024,
  expire: clock.add(Duration(days: days, hours: 5)).millisecondsSinceEpoch ~/ 1000,
  supportUrl: 'https://support.example.test',
  webPageUrl: 'https://sub.example.test/page',
  announce: '',
  fetchedAt: clock.millisecondsSinceEpoch ~/ 1000 - 1800,
  updateEvery: 3600,
);

class Scene {
  Scene({
    Connection connection = const Connection(),
    Subscription? subscription,
    List<Server> servers = const [],
    Map<String, Object?> settings = const {},
    bool ready = true,
    Traffic traffic = const Traffic(),
    String fault = '',
    Failure? failure,
  }) : transport = SilentTransport() {
    model = AppModel(CoreClient(transport))
      ..snapshot = Snapshot(
        version: '0.1.0',
        connection: connection,
        subscription: subscription,
        servers: servers,
        settings: settings,
      )
      ..ready = ready
      ..traffic = traffic
      ..fault = fault
      ..failure = failure;
  }

  final SilentTransport transport;
  late final AppModel model;

  Widget get app => SpofyApp(key: ObjectKey(this), model: model, now: () => clock);
}

Scene home(
  VpnState state, {
  String choice = autoChoice,
  String server = '',
  int delay = 0,
  String code = '',
  Map<String, Object?> settings = const {},
}) => Scene(
  connection: Connection(state: state, choice: choice, server: server, delayMs: delay, code: code),
  subscription: subscription(),
  servers: servers(),
  settings: settings,
  traffic: state.protecting ? const Traffic(up: 83 * 1024 * 1024, down: 1288490188) : const Traffic(),
);

Future<void> loadFonts() async {
  final bytes = File('fonts/Geologica.ttf').readAsBytesSync();
  final loader = FontLoader('Geologica')..addFont(Future.value(ByteData.sublistView(bytes)));
  await loader.load();
}

Future<void> show(WidgetTester tester, Scene scene, Size size, {double textScale = 1}) async {
  const ratio = 1.0;
  tester.view.devicePixelRatio = ratio;
  tester.view.physicalSize = size * ratio;
  tester.platformDispatcher.textScaleFactorTestValue = textScale;
  addTearDown(tester.view.reset);
  addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);
  await tester.pumpWidget(scene.app);
  await tester.pump(const Duration(milliseconds: 300));
}
