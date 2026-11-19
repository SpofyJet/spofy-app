import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/state/model.dart';
import 'package:spofy/ui/app.dart';
import 'package:spofy/ui/home.dart';
import 'package:spofy/ui/settings.dart';
import 'package:spofy/ui/strings.dart';
import 'package:spofy/ui/widgets.dart';

import 'scenes.dart';

const phone = Size(360, 800);
const small = Size(360, 640);
const tall = Size(390, 844);
const desktop = Size(1100, 720);

Future<void> golden(WidgetTester tester, String name) async {
  await expectLater(find.byType(SpofyApp), matchesGoldenFile('goldens/$name.png'));
}

Future<void> open(WidgetTester tester, String label) async {
  await tester.tap(find.bySemanticsLabel(RegExp('^$label')));
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 400));
}

void main() {
  setUpAll(loadFonts);

  final homes = <String, Scene Function()>{
    'home_loading': () => Scene(ready: false),
    'home_empty': () => Scene(),
    'home_idle': () => home(VpnState.idle),
    'home_idle_pinned': () => home(VpnState.idle, choice: 'de'),
    'home_measuring': () => home(VpnState.measuring),
    'home_connecting': () => home(VpnState.connecting, server: 'nl'),
    'home_connected': () => home(VpnState.connected, server: 'nl', delay: 38),
    'home_switching': () => home(VpnState.switching, server: 'de'),
    'home_stalled': () => home(VpnState.stalled, server: 'nl', code: 'no_server'),
    'home_offline': () => home(VpnState.offline, server: 'nl'),
    'home_failed': () => home(VpnState.failed, code: 'no_server'),
    'home_core_lost': () => Scene(ready: false, fault: AppModel.coreClosed),
    'home_denied': () => home(VpnState.idle)..model.failure = const Failure(Task.connect, AppModel.denied, ''),
    'home_refused': () => home(VpnState.failed, code: 'guard_failed'),
  };

  for (final entry in homes.entries) {
    testWidgets('${entry.key} on a phone', (tester) async {
      await show(tester, entry.value(), phone);
      await golden(tester, '${entry.key}_360');
    });
  }

  for (final (name, size) in [('small', small), ('tall', tall), ('desktop', desktop)]) {
    testWidgets('home connected, $name', (tester) async {
      await show(tester, home(VpnState.connected, server: 'nl', delay: 38), size);
      await golden(tester, 'home_connected_$name');
    });
  }

  testWidgets('the power control does not move between states and the loading placeholder matches it', (tester) async {
    Rect? first;
    for (final entry in homes.entries) {
      if (entry.key.contains('empty') || entry.key.contains('lost')) {
        continue;
      }
      await show(tester, entry.value(), phone);
      final control = find.byType(PowerControl);
      final circle = find.byWidgetPredicate(
        (w) =>
            w is DecoratedBox &&
            w.decoration is BoxDecoration &&
            (w.decoration as BoxDecoration).shape == BoxShape.circle,
      );
      final box = tester.getRect(control.evaluate().isEmpty ? circle.first : control);
      first ??= box;
      expect(box, first, reason: entry.key);
    }
  });

  testWidgets('the power control sits in the middle of a small phone and is big enough', (tester) async {
    await show(tester, home(VpnState.idle), small);
    final box = tester.getRect(find.byType(PowerControl));
    final share = box.center.dy / small.height;
    printOnFailure('power control center at ${(share * 100).round()} % of the height');
    expect(share, inInclusiveRange(0.38, 0.62));
    expect(box.width, greaterThanOrEqualTo(120));
    for (final tile in tester.widgetList<Tile>(find.byType(Tile))) {
      expect(tester.getSize(find.byWidget(tile)).height, greaterThanOrEqualTo(48));
    }
  });

  testWidgets('nothing overflows with large text on a small phone', (tester) async {
    for (final scene in homes.values) {
      await show(tester, scene(), small, textScale: 1.5);
      expect(tester.takeException(), isNull);
    }
    await show(tester, home(VpnState.stalled, server: 'nl'), const Size(320, 480), textScale: 1.5);
    expect(tester.takeException(), isNull);
  });

  testWidgets('no text is cut on the home screen, down to a 320 px phone with 130 % text', (tester) async {
    for (final (size, scale) in [
      (phone, 1.0),
      (small, 1.0),
      (const Size(320, 568), 1.0),
      (const Size(320, 568), 1.3),
    ]) {
      for (final entry in homes.entries) {
        await show(tester, entry.value(), size, textScale: scale);
        for (final text in tester.renderObjectList<RenderParagraph>(find.byType(RichText))) {
          expect(text.didExceedMaxLines, isFalse, reason: '${entry.key} at $size × $scale: ${text.text.toPlainText()}');
        }
      }
    }
  });

  testWidgets('servers', (tester) async {
    await show(tester, home(VpnState.idle), phone);
    await open(tester, S.server);
    await golden(tester, 'servers_360');
    expect(find.text('Обход 1'), findsOneWidget);
    final names = tester.widgetList<Tile>(find.byType(Tile)).map((t) => t.title).toList();
    expect(names.take(4), ['Авто', 'Обход 1', 'Нидерланды', 'Германия']);
  });

  testWidgets('servers on a desktop window', (tester) async {
    await show(tester, home(VpnState.connected, server: 'nl', delay: 38, choice: 'nl'), desktop);
    await open(tester, S.server);
    await golden(tester, 'servers_desktop');
  });

  testWidgets('choosing a server asks the core to connect to it and returns home', (tester) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.server);
    await tester.tap(find.text('Германия'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(scene.transport.sent.single, '{"id":1,"op":"connect","server":"de"}');
    expect(find.byType(PowerControl), findsOneWidget);
    expect(scene.model.connection.choice, 'de');
  });

  testWidgets('measuring while the tunnel is up is asked from the core and its refusal is explained', (tester) async {
    final scene = home(VpnState.connected, server: 'nl', delay: 38);
    await show(tester, scene, phone);
    await open(tester, S.server);
    scene.transport.sent.clear();
    await tester.tap(find.bySemanticsLabel(S.measure));
    await tester.pump();
    expect(scene.transport.sent.where((line) => line.contains('"op":"measure"')), hasLength(1));
    scene.model
      ..failure = const Failure(Task.measure, 'connected', '')
      ..notifyListeners();
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text(S.reason('connected')), findsOneWidget);
  });

  testWidgets('subscription details', (tester) async {
    await show(tester, home(VpnState.idle), phone);
    await open(tester, S.subscription);
    await golden(tester, 'subscription_360');
  });

  testWidgets('subscription that ends soon and is almost used up', (tester) async {
    final scene = Scene(subscription: subscription(days: 2, used: 96000), servers: servers());
    await show(tester, scene, phone);
    await golden(tester, 'home_ending_360');
    await open(tester, S.subscription);
    await golden(tester, 'subscription_ending_360');
  });

  testWidgets('adding a subscription', (tester) async {
    final scene = Scene();
    await show(tester, scene, phone);
    await open(tester, S.manual);
    await golden(tester, 'subscription_add_360');
    await tester.enterText(find.byType(EditableText), ' https://sub.example.test/sub/TESTTOKEN0123456789 ');
    await tester.tap(find.bySemanticsLabel(S.add));
    await tester.pump();
    expect(
      scene.transport.sent.single,
      '{"id":1,"op":"subscribe","url":"https://sub.example.test/sub/TESTTOKEN0123456789"}',
    );
    await tester.pump(const Duration(milliseconds: 300));
    await golden(tester, 'subscription_adding_360');
    scene.model
      ..failure = const Failure(Task.subscribe, 'sub_status', 'status 403')
      ..notifyListeners();
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text(S.reason('sub_status')), findsOneWidget);
  });

  testWidgets('removing a subscription asks first', (tester) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.subscription);
    await tester.tap(find.bySemanticsLabel(S.remove));
    await tester.pump();
    expect(scene.transport.sent, isEmpty);
    expect(find.text(S.removeConfirm), findsOneWidget);
    await golden(tester, 'subscription_remove_360');
    await tester.tap(find.bySemanticsLabel(S.removeShort));
    await tester.pump();
    expect(scene.transport.sent.single, '{"id":1,"op":"unsubscribe"}');
  });

  testWidgets('the first launch takes the link from the clipboard in one tap', (tester) async {
    String? clipboard;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
      return call.method == 'Clipboard.getData' && clipboard != null ? {'text': clipboard} : null;
    });
    addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, null));
    final scene = Scene();
    await show(tester, scene, phone);
    for (final junk in [null, '', 'привет', 'http://sub.example.test/x', 'https://a.test/x и ещё текст']) {
      clipboard = junk;
      await tester.tap(find.bySemanticsLabel(S.pasteLink));
      await tester.pump();
      await tester.pump();
      expect(scene.transport.sent, isEmpty, reason: '$junk');
      expect(find.textContaining(S.noLink, findRichText: true), findsOneWidget, reason: '$junk');
    }
    await golden(tester, 'home_empty_no_link_360');
    clipboard = ' https://sub.example.test/sub/TESTTOKEN0123456789\n';
    await tester.tap(find.bySemanticsLabel(S.pasteLink));
    await tester.pump();
    await tester.pump();
    expect(
      scene.transport.sent.single,
      '{"id":1,"op":"subscribe","url":"https://sub.example.test/sub/TESTTOKEN0123456789"}',
    );
    expect(find.textContaining(S.noLink, findRichText: true), findsNothing);
  });

  testWidgets('settings show what the core holds and ask the core to change it', (tester) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await golden(tester, 'settings_360');
    expect(tester.widgetList<Toggle>(find.byType(Toggle)).map((t) => t.value), [false, false]);
    await tester.tap(find.bySemanticsLabel(S.autoConnect));
    await tester.tap(find.bySemanticsLabel(S.killSwitch));
    await tester.pump();
    expect(scene.transport.sent, [
      '{"id":1,"op":"set","key":"autoConnect","value":true}',
      '{"id":2,"op":"set","key":"killSwitch","value":true}',
    ]);
    expect(tester.widgetList<Toggle>(find.byType(Toggle)).map((t) => t.value), [false, false]);

    final on = home(VpnState.idle, settings: {autoConnectKey: true, killSwitchKey: true});
    await show(tester, on, phone);
    await open(tester, S.settings);
    await golden(tester, 'settings_on_360');
    expect(tester.widgetList<Toggle>(find.byType(Toggle)).map((t) => t.value), [true, true]);
    await tester.tap(find.bySemanticsLabel(S.killSwitch));
    await tester.pump();
    expect(on.transport.sent.single, '{"id":1,"op":"set","key":"killSwitch","value":false}');
    expect(find.text(S.version('0.1.0')), findsOneWidget);
  });

  testWidgets('settings fit a 320 px phone with 130 % text', (tester) async {
    await show(tester, home(VpnState.idle, always: () {}), const Size(320, 568), textScale: 1.3);
    await open(tester, S.settings);
    expect(tester.takeException(), isNull);
    expect(find.text(S.alwaysOn), findsOneWidget);
    for (final text in tester.renderObjectList<RenderParagraph>(find.byType(RichText))) {
      expect(text.didExceedMaxLines, isFalse, reason: text.text.toPlainText());
    }
  });

  testWidgets('where the system can hold the VPN itself the settings lead to it', (tester) async {
    var opened = 0;
    await show(tester, home(VpnState.idle, always: () => opened++), phone);
    await open(tester, S.settings);
    await golden(tester, 'settings_system_360');
    await tester.tap(find.bySemanticsLabel(RegExp('^${S.alwaysOn}')));
    expect(opened, 1);
    await show(tester, home(VpnState.idle), phone);
    await open(tester, S.settings);
    expect(find.text(S.alwaysOn), findsNothing);
  });

  testWidgets('the power control sends connect, then disconnect', (tester) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await tester.tap(find.bySemanticsLabel(S.connect));
    await tester.pump();
    expect(scene.transport.sent.single, '{"id":1,"op":"connect"}');
    final connected = home(VpnState.connected, server: 'nl', delay: 38);
    await show(tester, connected, phone);
    connected.transport.sent.clear();
    await tester.tap(find.bySemanticsLabel(S.disconnect));
    await tester.pump();
    expect(connected.transport.sent.single, contains('"op":"disconnect"'));
  });

  testWidgets('traffic is polled only while the tunnel is up and the app is visible', (tester) async {
    final idle = home(VpnState.idle);
    await show(tester, idle, phone);
    await tester.pump(const Duration(seconds: 7));
    expect(idle.transport.sent, isEmpty);
    final connected = home(VpnState.connected, server: 'nl', delay: 38);
    await show(tester, connected, phone);
    await tester.pump(const Duration(seconds: 6));
    final polls = connected.transport.sent.where((line) => line.contains('"traffic"')).length;
    expect(polls, inInclusiveRange(3, 5));
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    connected.transport.sent.clear();
    await tester.pump(const Duration(seconds: 10));
    expect(connected.transport.sent, isEmpty);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pump(const Duration(seconds: 1));
    expect(connected.transport.sent.first, contains('"op":"check"'));
  });

  test('numbers and dates read naturally', () {
    expect(S.bytes(0), '0 Б');
    expect(S.bytes(1536), '1,5 КБ');
    expect(S.bytes(1288490188), '1,2 ГБ');
    expect(S.bytes(107374182400), '100 ГБ');
    expect(S.bytes(5 * 1024 * 1024 * 1024), '5 ГБ');
    expect([1, 2, 5, 11, 21, 22, 25, 111].map(S.days).toList(), [
      '1 день',
      '2 дня',
      '5 дней',
      '11 дней',
      '21 день',
      '22 дня',
      '25 дней',
      '111 дней',
    ]);
    expect(S.until(subscription(days: 42), clock), 'до 31 дек');
    expect(S.until(subscription(days: 60), clock), 'до 18 янв 2027');
    expect(S.until(subscription(days: 3), clock), 'ещё 3 дня');
    expect(S.until(subscription(days: 0), clock), 'до конца дня');
    expect(S.until(subscription(days: -2), clock), 'закончилась');
    expect(S.usage(subscription(used: 96000)), '93,8 из 100 ГБ');
    expect(S.usage(subscription(used: 1100)), '1,1 из 100 ГБ');
    expect(S.usage(subscription(used: 1100, total: 0)), 'Без ограничений');
    expect(S.expiry(subscription(days: 42), clock), 'До 31 декабря');
    expect(S.expiry(subscription(days: 60), clock), 'До 18 января 2027');
    expect(S.expiry(subscription(days: 3), clock), 'Ещё 3 дня');
    expect(S.expiry(subscription(days: 0), clock), 'Заканчивается сегодня');
    expect(S.expiry(subscription(days: -2), clock), 'Закончилась');
    final lifetime = Subscription(
      title: '',
      upload: 0,
      download: 0,
      total: 0,
      expire: DateTime.utc(2099, 12, 31).millisecondsSinceEpoch ~/ 1000,
      supportUrl: '',
      webPageUrl: '',
      announce: '',
      fetchedAt: 0,
      updateEvery: 0,
    );
    expect(S.until(lifetime, clock), 'навсегда');
    expect(S.usage(lifetime), 'Без ограничений');
    expect(S.usage(subscription()), '13 из 100 ГБ');
  });
}
