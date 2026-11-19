import 'dart:io';

import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/state/model.dart';
import 'package:spofy/ui/strings.dart';
import 'package:spofy/ui/theme.dart';
import 'package:spofy/ui/widgets.dart';

import '../test/scenes.dart';

Future<void> sheet(
  WidgetTester tester,
  String name,
  List<(String, Scene, String?)> frames, {
  Size frame = const Size(360, 800),
  double scale = 1,
}) async {
  final out = Platform.environment['SPOFY_SHEETS'] ?? 'build/sheets';
  Directory(out).createSync(recursive: true);
  final size = Size(24 + frames.length * (frame.width + 24), frame.height + 76);
  tester.view.devicePixelRatio = 1;
  tester.view.physicalSize = size;
  addTearDown(tester.view.reset);
  final keys = [for (final _ in frames) GlobalKey()];
  await tester.pumpWidget(
    Directionality(
      textDirection: TextDirection.ltr,
      child: ColoredBox(
        color: const Color(0xFF1E222C),
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              for (var i = 0; i < frames.length; i++) ...[
                if (i > 0) const SizedBox(width: 24),
                Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    ClipRRect(
                      borderRadius: const BorderRadius.all(Radius.circular(18)),
                      child: SizedBox.fromSize(
                        key: keys[i],
                        size: frame,
                        child: MediaQuery(
                          data: MediaQueryData(size: frame, textScaler: TextScaler.linear(scale)),
                          child: frames[i].$2.app,
                        ),
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(frames[i].$1, style: Ink.type(14, 500, const Color(0xFFC8CDD7))),
                  ],
                ),
              ],
            ],
          ),
        ),
      ),
    ),
  );
  await tester.pump(const Duration(milliseconds: 300));
  for (var i = 0; i < frames.length; i++) {
    final label = frames[i].$3;
    if (label != null) {
      for (final step in label.split('|')) {
        await tester.tap(
          find.descendant(
            of: find.byKey(keys[i]),
            matching: find.byWidgetPredicate((w) => w is Pressable && w.label.startsWith(step)),
          ),
        );
        await tester.pump();
        await tester.pump(const Duration(milliseconds: 400));
      }
    }
  }
  await expectLater(find.byType(Directionality).first, matchesGoldenFile('${Directory(out).absolute.path}/$name.png'));
}

void main() {
  setUpAll(loadFonts);

  testWidgets('home', (tester) async {
    await sheet(tester, '1-home', [
      ('VPN выключен', home(VpnState.idle), null),
      ('Подключение', home(VpnState.connecting, server: 'nl'), null),
      ('VPN работает', home(VpnState.connected, server: 'nl', delay: 38), null),
      ('Сервер выбран вручную', home(VpnState.connected, server: 'de', delay: 44, choice: 'de'), null),
    ]);
  });

  testWidgets('trouble', (tester) async {
    await sheet(tester, '2-trouble', [
      ('Сервер замолчал', home(VpnState.stalled, server: 'nl', code: 'no_server', guarded: true), null),
      ('Нет интернета', home(VpnState.offline, server: 'nl'), null),
      ('Не подключились', home(VpnState.failed, code: 'no_server'), null),
      ('Подписка заканчивается', Scene(subscription: subscription(days: 2, used: 96000), servers: servers()), null),
    ]);
  });

  testWidgets('start', (tester) async {
    await sheet(tester, '3-start', [
      ('Первый запуск', Scene(), null),
      ('Добавление подписки', Scene(), S.manual),
      ('Ссылка не подошла', Scene(failure: const Failure(Task.subscribe, 'sub_status', '')), null),
      ('Загрузка', Scene(ready: false), null),
    ]);
  });

  testWidgets('small', (tester) async {
    await sheet(tester, '5-small', frame: const Size(360, 640), [
      ('360 × 640', home(VpnState.connected, server: 'nl', delay: 38), null),
      ('Не подключились', home(VpnState.failed, code: 'no_server'), null),
      ('В буфере не ссылка', Scene(failure: const Failure(Task.subscribe, 'sub_bad_url', '')), null),
      ('Серверы', home(VpnState.idle), S.server),
    ]);
  });

  testWidgets('large text', (tester) async {
    await sheet(tester, '6-large-text', frame: const Size(320, 568), scale: 1.3, [
      ('320 × 568, шрифт 130 %', home(VpnState.connected, server: 'nl', delay: 38), null),
      ('Сервер замолчал', home(VpnState.stalled, server: 'nl', code: 'no_server', guarded: true), null),
      ('Первый запуск', Scene(), null),
      ('Подписка', Scene(subscription: subscription(days: 2, used: 96000), servers: servers()), S.subscription),
    ]);
  });

  testWidgets('inner', (tester) async {
    await sheet(tester, '4-inner', [
      ('Серверы', home(VpnState.idle), S.server),
      ('Серверы, выбран вручную', home(VpnState.connected, server: 'de', delay: 44, choice: 'de'), S.server),
      ('Подписка', home(VpnState.idle), S.subscription),
      ('Удаление подписки', home(VpnState.idle), '${S.subscription}|${S.remove}'),
    ]);
  });
}
