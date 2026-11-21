import 'dart:async';

import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/client.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/state/apps.dart';
import 'package:spofy/state/model.dart';
import 'package:spofy/ui/app.dart';
import 'package:spofy/ui/home.dart';
import 'package:spofy/ui/servers.dart';
import 'package:spofy/ui/settings.dart';
import 'package:spofy/ui/strings.dart';
import 'package:spofy/ui/subscription.dart';
import 'package:spofy/ui/widgets.dart';

import 'replay.dart';
import 'scenes.dart';
import 'transcript.dart';

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
    'home_ended': () => stopped('ended', days: -3),
    'home_spent': () => stopped('spent', used: 102400),
    'home_devices': () => stopped('devices'),
    'home_other': () => stopped('other', notes: const ['🚫 Подписка отключена', '👉 Напишите в поддержку']),
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
      final scene = entry.value();
      if (entry.key.contains('empty') || entry.key.contains('lost') || (scene.model.subscription?.stopped ?? false)) {
        continue;
      }
      await show(tester, scene, phone);
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
    expect(rows(tester).take(4), ['Обход', 'Нидерланды', 'Германия', 'Финляндия']);
  });

  testWidgets('servers on a desktop window', (tester) async {
    await show(tester, home(VpnState.connected, server: 'nl', delay: 38, choice: 'nl'), desktop);
    await open(tester, S.server);
    await golden(tester, 'servers_desktop');
  });

  testWidgets('notes that come with the servers stand after them as one row that opens what they name', (tester) async {
    const notes = [
      '🚫 В вашем тарифе нет этих серверов',
      '🏴‍☠️ Они в тарифе «Больше»',
      '👉 Сменить тариф в боте',
      '🤖 @example_bot',
    ];
    final opened = <String>[];
    final scene = Scene(
      subscription: subscription(notes: notes),
      servers: servers(),
      browse: opened.add,
    );
    await show(tester, scene, phone);
    await open(tester, S.server);
    final title = find.text('В вашем тарифе нет этих серверов');
    await tester.scrollUntilVisible(
      title,
      200,
      scrollable: find.descendant(
        of: find.byType(ServersScreen),
        matching: find.byWidgetPredicate((w) => w is Scrollable && w.axisDirection == AxisDirection.down),
      ),
    );
    await tester.pump(const Duration(milliseconds: 300));
    await golden(tester, 'servers_notes_360');
    expect(
      find.textContaining('Они в тарифе «Больше»\nСменить тариф в боте\n@example_bot', findRichText: true),
      findsOneWidget,
    );
    expect(rows(tester).where((t) => t == 'Сингапур'), hasLength(1));
    await tester.tap(title);
    await tester.pump();
    expect(opened, ['https://t.me/example_bot']);
    expect(scene.transport.sent.where((line) => line.contains('"op":"connect"')), isEmpty);
  });

  testWidgets('notes without a link open the subscription page, and nothing is offered when no page can be opened', (
    tester,
  ) async {
    for (final (page, browse, want) in [
      ('https://sub.example.test/page', true, 'https://sub.example.test/page'),
      ('http://sub.example.test/page', true, null),
      ('https://sub.example.test/page', false, null),
    ]) {
      final opened = <String>[];
      final scene = Scene(
        subscription: subscription(notes: const ['⛔ Трафик на эти серверы закончился'], page: page),
        servers: servers().take(2).toList(),
        browse: browse ? opened.add : null,
      );
      await show(tester, scene, phone);
      await open(tester, S.server);
      final row = find.text('Трафик на эти серверы закончился');
      expect(row, findsOneWidget);
      expect(find.ancestor(of: row, matching: find.byType(Pressable)), want == null ? findsNothing : findsOneWidget);
      if (want != null) {
        await tester.tap(row);
        expect(opened, [want]);
      }
    }
  });

  test('a note keeps its own words: only the marks in front are dropped, and a link is found in it', () {
    expect(S.said(['🤖 @example_bot', '🏴‍☠️ Они «в» тарифе', '  ', '✅', '1️⃣ Первый']), [
      '@example_bot',
      'Они «в» тарифе',
      '1️⃣ Первый',
    ]);
    expect(noteLink(['Нет ссылки', 'Пишите @example_bot']), 'https://t.me/example_bot');
    expect(noteLink(['Продлить: web.example.test/pay']), 'https://web.example.test/pay');
    expect(noteLink(['Продлить: https://web.example.test']), 'https://web.example.test');
    expect(noteLink(['@abc слишком коротко', 'и т.д.']), null);
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
    final opened = <String>[];
    await show(tester, home(VpnState.idle, browse: opened.add), phone);
    await open(tester, S.subscription);
    await golden(tester, 'subscription_360');
    await tester.tap(find.bySemanticsLabel(S.renew));
    expect(opened, ['https://sub.example.test/page']);
  });

  testWidgets('subscription that ends soon and is almost used up', (tester) async {
    final scene = Scene(subscription: subscription(days: 2, used: 96000), servers: servers(), browse: (_) {});
    await show(tester, scene, phone);
    await golden(tester, 'home_ending_360');
    await open(tester, S.subscription);
    await golden(tester, 'subscription_ending_360');
  });

  testWidgets('a subscription that ended: the way to pay is the one bright button and nothing offers to connect', (
    tester,
  ) async {
    final opened = <String>[];
    final scene = Scene(
      subscription: subscription(days: -3, stop: 'ended'),
      browse: opened.add,
    );
    await show(tester, scene, phone);
    expect(find.text('Подписка закончилась'), findsOneWidget);
    expect(find.byType(PowerControl), findsNothing);
    expect(find.byType(PrimaryButton), findsOneWidget);
    await tester.tap(find.bySemanticsLabel(S.renew));
    expect(opened, ['https://sub.example.test/page']);
    await tester.tap(find.bySemanticsLabel(S.checkAgain));
    await tester.pump();
    expect(scene.transport.sent.where((line) => line.contains('"op":"refresh"')), hasLength(1));
    scene.transport.answer('{"re":1,"ok":true}');
    await tester.pump();
    await open(tester, '${S.subscription},');
    await golden(tester, 'subscription_ended_360');
    expect(find.text('Закончилась'), findsOneWidget);
    expect(find.text('16 ноября'), findsOneWidget);
    expect(find.bySemanticsLabel(S.renew), findsOneWidget);
  });

  testWidgets('back from the browser the list is asked again, and only while the subscription gives no servers', (
    tester,
  ) async {
    for (final (scene, asks) in [(stopped('ended', days: -3), 1), (home(VpnState.idle), 0)]) {
      await show(tester, scene, phone);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      await tester.pump();
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pump();
      expect(scene.transport.sent.where((line) => line.contains('"op":"refresh"')), hasLength(asks));
    }
  });

  testWidgets('a page that cannot be opened is not offered, and each reason has its own words and button', (
    tester,
  ) async {
    for (final scene in [
      stopped('ended', days: -3, pay: false),
      Scene(
        subscription: subscription(days: -3, stop: 'ended', page: 'http://sub.example.test/page'),
        browse: (_) {},
      ),
      Scene(
        subscription: subscription(days: -3, stop: 'ended', page: ''),
        browse: (_) {},
      ),
    ]) {
      await show(tester, scene, phone);
      expect(find.byType(PrimaryButton), findsNothing);
      expect(find.bySemanticsLabel(S.checkAgain), findsOneWidget);
    }
    final words = <String>{};
    for (final why in ['ended', 'spent', 'devices', 'other']) {
      final s = subscription(stop: why, notes: const ['⏰ Первая строка', '   ', '👉 Вторая строка']);
      words.addAll([S.stopTitle(s), S.stopHint(s), S.stopAction(s)]);
    }
    expect(words, hasLength(12));
    expect(
      S.stopHint(subscription(stop: 'other', notes: const ['⏰ Первая строка', '👉 Вторая'])),
      'Первая строка. Вторая',
    );
    expect(S.stopHint(subscription(stop: 'other')), isNotEmpty);
    expect(S.pageAction(subscription()), S.renew);
    expect(S.pageAction(subscription(days: 30000)), S.manage);
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
    expect(tester.widgetList<Toggle>(find.byType(Toggle)).map((t) => t.value).take(2), [false, false]);
    await tester.tap(find.bySemanticsLabel(S.autoConnect));
    await tester.tap(find.bySemanticsLabel(S.killSwitch));
    await tester.pump();
    expect(scene.transport.sent, [
      '{"id":1,"op":"set","key":"autoConnect","value":true}',
      '{"id":2,"op":"set","key":"killSwitch","value":true}',
    ]);
    expect(tester.widgetList<Toggle>(find.byType(Toggle)).map((t) => t.value).take(2), [false, false]);

    final on = home(VpnState.idle, settings: {autoConnectKey: true, killSwitchKey: true});
    await show(tester, on, phone);
    await open(tester, S.settings);
    await golden(tester, 'settings_on_360');
    expect(tester.widgetList<Toggle>(find.byType(Toggle)).map((t) => t.value).take(2), [true, true]);
    await tester.tap(find.bySemanticsLabel(S.killSwitch));
    await tester.pump();
    expect(on.transport.sent.single, '{"id":1,"op":"set","key":"killSwitch","value":false}');
    await tester.scrollUntilVisible(find.text(S.version('0.1.0')), 200);
    expect(find.text(S.version('0.1.0')), findsOneWidget);
  });

  testWidgets('an app link opens the form with the address; only the person adds it', (tester) async {
    const address = 'https://sub.example.test/sub/TESTTOKEN0123456789';
    final scene = Scene();
    await show(tester, scene, phone);
    scene.model.offer('spofy://add/$address');
    await tester.pump();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text(S.subscription), findsOneWidget);
    expect(tester.widget<EditableText>(find.byType(EditableText)).controller.text, address);
    expect(scene.model.offered, isNull);
    expect(scene.transport.sent, isEmpty);
    expect(find.text(S.offeredHint), findsOneWidget);
    await golden(tester, 'subscription_offered_360');
    await tester.tap(find.bySemanticsLabel(S.add));
    await tester.pump();
    expect(scene.transport.sent, ['{"id":1,"op":"subscribe","url":"$address"}']);
  });

  testWidgets('an app link with a subscription in place asks before replacing it', (tester) async {
    const address = 'https://other.example.test/sub/TESTTOKEN0123456789';
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    scene.model.offer('spofy://add/$address');
    await tester.pump();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text(S.replaceConfirm), findsOneWidget);
    expect(find.text(S.replaceFrom(address)), findsOneWidget);
    expect(S.replaceFrom(address), contains('other.example.test'));
    await golden(tester, 'subscription_replace_360');
    await tester.tap(find.bySemanticsLabel(S.cancel));
    await tester.pump();
    expect(find.text(S.replaceConfirm), findsNothing);
    expect(scene.transport.sent, isEmpty);
    scene.model.offer('spofy://add/$address');
    await tester.pump();
    expect(find.text(S.replaceConfirm), findsOneWidget);
    expect(find.byType(SubscriptionScreen), findsOneWidget);
    await tester.tap(find.bySemanticsLabel(S.replace));
    await tester.pump();
    expect(scene.transport.sent, ['{"id":1,"op":"subscribe","url":"$address"}']);
  });

  testWidgets('a link that came before the core answered waits and then asks before replacing', (tester) async {
    const address = 'https://other.example.test/sub/TESTTOKEN0123456789';
    final hello = Transcript.load().exchanges.firstWhere(
      (e) =>
          e.sent.contains('"hello"') &&
          e.received.last.contains('"subscription":{') &&
          e.received.last.contains('"update":null'),
    );
    final transport = ReplayTransport([hello]);
    final model = AppModel(CoreClient(transport));
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = phone;
    addTearDown(tester.view.reset);
    system(tester);
    await tester.pumpWidget(SpofyApp(model: model, now: () => clock));
    model.offer('spofy://add/$address');
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.byType(SubscriptionScreen), findsNothing);
    expect(model.offered, address);
    unawaited(model.start());
    await tester.pump();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(model.ready, isTrue);
    expect(find.text(S.replaceConfirm), findsOneWidget);
    expect(find.text(S.replaceFrom(address)), findsOneWidget);
    expect(transport.position, 1);
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

  testWidgets('apps: every app goes through the VPN until the person says otherwise', (tester) async {
    final scene = home(VpnState.idle, apps: FakeApps());
    await show(tester, scene, phone);
    await open(tester, S.settings);
    expect(find.text(S.appsSummary(const AppsChoice())), findsOneWidget);
    await golden(tester, 'settings_apps_360');
    await open(tester, S.apps);
    await golden(tester, 'apps_all_360');
    expect(find.text(S.appsHint(AppsMode.all)), findsOneWidget);
    expect(find.byType(EditableText), findsNothing);
    expect(find.text('Банк'), findsNothing);
    await tester.pump(const Duration(seconds: 1));
    expect(scene.transport.sent, isEmpty);
    await show(tester, home(VpnState.idle), phone);
    await open(tester, S.settings);
    expect(find.text(S.apps), findsNothing);
  });

  testWidgets(
    'apps taken out of the VPN are sent once, after the last tap, and icons are asked only for rows on screen',
    (tester) async {
      final apps = FakeApps();
      final scene = home(VpnState.idle, apps: apps);
      await show(tester, scene, phone);
      await open(tester, S.settings);
      await open(tester, S.apps);
      await tester.tap(find.bySemanticsLabel(S.appsExcept));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      expect(find.text(S.appsHint(AppsMode.except)), findsOneWidget);
      await tester.tap(find.bySemanticsLabel('Маркет'));
      await tester.pump(const Duration(milliseconds: 300));
      await tester.tap(find.bySemanticsLabel('Банк'));
      await tester.pump(const Duration(milliseconds: 300));
      expect(scene.transport.sent, isEmpty);
      await golden(tester, 'apps_except_360');
      await tester.pump(const Duration(milliseconds: 500));
      expect(scene.transport.sent, [
        '{"id":1,"op":"set","key":"apps","value":{"mode":"except","list":["org.example.bank","org.example.market"]}}',
      ]);
      expect(apps.icons, isNotEmpty);
      expect(apps.icons.length, lessThan(phoneApps.length));
      expect(apps.icons.toSet().length, apps.icons.length);
      await tester.tap(find.bySemanticsLabel('Банк'));
      await tester.tap(find.bySemanticsLabel(S.back));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 400));
      expect(
        scene.transport.sent.last,
        '{"id":2,"op":"set","key":"apps","value":{"mode":"except","list":["org.example.market"]}}',
      );
      expect(scene.transport.sent, hasLength(2));
    },
  );

  testWidgets('only the chosen apps: nothing is sent until one is marked', (tester) async {
    final scene = home(VpnState.idle, apps: FakeApps());
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await open(tester, S.apps);
    await tester.tap(find.bySemanticsLabel(S.appsOnly));
    await tester.pump();
    await tester.pump(const Duration(seconds: 1));
    expect(find.text(S.appsOnlyEmpty), findsOneWidget);
    expect(scene.transport.sent, isEmpty);
    await golden(tester, 'apps_only_empty_360');
    await tester.tap(find.bySemanticsLabel('Браузер'));
    await tester.pump();
    expect(find.text(S.appsOnlyEmpty), findsNothing);
    await tester.pump(const Duration(seconds: 1));
    expect(
      scene.transport.sent.single,
      '{"id":1,"op":"set","key":"apps","value":{"mode":"only","list":["org.example.browser"]}}',
    );
  });

  testWidgets('a saved choice opens with its apps first; one that is gone leaves the list only with a change', (
    tester,
  ) async {
    final saved = {
      appsKey: {
        'mode': 'except',
        'list': ['org.example.gone', 'org.example.taxi'],
      },
    };
    final scene = home(VpnState.idle, settings: saved, apps: FakeApps());
    await show(tester, scene, phone);
    await open(tester, S.settings);
    expect(find.text(S.appsSummary(AppsChoice.of(saved[appsKey]))), findsOneWidget);
    await open(tester, S.apps);
    final rows = tester.widgetList<Tile>(find.byType(Tile)).toList();
    expect(rows.first.title, 'Такси');
    expect(rows.first.selected, isTrue);
    expect(rows[1].title, 'Calculator');
    expect(rows.where((row) => row.selected), hasLength(1));
    await tester.tap(find.bySemanticsLabel(S.back));
    await tester.pump();
    await tester.pump(const Duration(seconds: 1));
    expect(scene.transport.sent, isEmpty);
    await open(tester, S.apps);
    await tester.tap(find.bySemanticsLabel(S.appsAll));
    await tester.pump();
    await tester.pump(const Duration(seconds: 1));
    expect(
      scene.transport.sent.single,
      '{"id":1,"op":"set","key":"apps","value":{"mode":"all","list":["org.example.taxi"]}}',
    );
    expect(find.byType(Tile), findsNothing);
  });

  testWidgets('search narrows the list by name and says when nothing matches', (tester) async {
    final scene = home(
      VpnState.idle,
      settings: {
        appsKey: {
          'mode': 'only',
          'list': ['org.example.mail'],
        },
      },
      apps: FakeApps(),
    );
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await open(tester, S.apps);
    await tester.enterText(find.byType(EditableText), ' бАн ');
    await tester.pump();
    expect(tester.widgetList<Tile>(find.byType(Tile)).map((row) => row.title), ['Банк']);
    await tester.enterText(find.byType(EditableText), 'example.ma');
    await tester.pump();
    expect(tester.widgetList<Tile>(find.byType(Tile)).map((row) => row.title), ['Почта', 'Карты', 'Маркет']);
    await tester.enterText(find.byType(EditableText), 'нет такого');
    await tester.pump();
    expect(find.byType(Tile), findsNothing);
    expect(find.text(S.appsNone), findsOneWidget);
    await tester.tap(find.bySemanticsLabel(S.appsClear));
    await tester.pump();
    expect(tester.widgetList<Tile>(find.byType(Tile)).first.title, 'Почта');
    expect(scene.transport.sent, isEmpty);
  });

  testWidgets('when the system gives no list the screen says so and the mode can still be changed', (tester) async {
    final scene = home(
      VpnState.idle,
      settings: {
        appsKey: {
          'mode': 'except',
          'list': ['org.example.bank'],
        },
      },
      apps: FakeApps(broken: true),
    );
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await open(tester, S.apps);
    expect(find.text(S.appsFailed), findsOneWidget);
    expect(find.byType(Tile), findsNothing);
    await tester.tap(find.bySemanticsLabel(S.appsAll));
    await tester.pump();
    await tester.pump(const Duration(seconds: 1));
    expect(
      scene.transport.sent.single,
      '{"id":1,"op":"set","key":"apps","value":{"mode":"all","list":["org.example.bank"]}}',
    );
  });

  testWidgets('the system blocks traffic outside the VPN: the apps screen says what that does to the choice', (
    tester,
  ) async {
    final scene = home(
      VpnState.idle,
      settings: {
        appsKey: {
          'mode': 'except',
          'list': ['org.example.bank'],
        },
      },
      apps: FakeApps(lockdown: true),
    );
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await open(tester, S.apps);
    expect(find.text(S.appsLocked), findsOneWidget);
    await golden(tester, 'apps_locked_360');
    await tester.tap(find.bySemanticsLabel(S.appsAll));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text(S.appsLocked), findsNothing);
    await tester.pump(const Duration(seconds: 1));
  });

  testWidgets('the system does not block traffic outside the VPN: no notice', (tester) async {
    final scene = home(
      VpnState.idle,
      settings: {
        appsKey: {
          'mode': 'except',
          'list': ['org.example.bank'],
        },
      },
      apps: FakeApps(),
    );
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await open(tester, S.apps);
    expect(find.byType(Tile), findsWidgets);
    expect(find.text(S.appsLocked), findsNothing);
  });

  testWidgets('support: the report shows what the core knows, is copied whole and holds no address', (tester) async {
    final opened = <String>[];
    final copied = <String>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
      if (call.method == 'Clipboard.setData') {
        copied.add((call.arguments as Map<Object?, Object?>)['text']! as String);
      }
      return null;
    });
    addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, null));
    final scene = home(VpnState.stalled, server: 'de', code: 'server_unreachable', browse: opened.add);
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await open(tester, S.support);
    expect(scene.transport.sent.last, '{"id":2,"op":"report"}');
    expect(find.textContaining('Журнал'), findsNothing);
    scene.transport.answer(
      '{"re":2,"ok":true,"report":{"os":"Android","osVersion":"15","model":"Pixel 8","device":"hw-1","startedAt":1795089600,'
      '"journal":[{"at":1795089960,"state":"connected","server":"nl","code":""},'
      '{"at":1795090440,"state":"stalled","server":"de","code":"server_unreachable"}]}}',
    );
    await tester.pump();
    await tester.pump();
    await golden(tester, 'support_360');
    await tester.tap(find.bySemanticsLabel(S.copyReport));
    await tester.pump();
    expect(find.bySemanticsLabel(S.copied), findsOneWidget);
    final text = copied.single;
    for (final part in [
      'Spofy 0.1.0',
      'Android 15 · Pixel 8',
      'hw-1',
      'Сервер не отвечает',
      'server_unreachable',
      '19.11 12:06:00 connected',
      'Журнал',
    ]) {
      expect(text, contains(part));
    }
    for (final secret in ['https://', 'example.test', 'sub.']) {
      expect(text, isNot(contains(secret)));
    }
    await tester.pump(const Duration(seconds: 3));
    expect(find.bySemanticsLabel(S.copyReport), findsOneWidget);
    await tester.tap(find.bySemanticsLabel(S.writeSupport));
    expect(opened, ['https://support.example.test']);
  });

  testWidgets('support fits a 320 px phone with 130 % text and hides the link it cannot open', (tester) async {
    final scene = home(VpnState.connected, server: 'de', delay: 34);
    await show(tester, scene, const Size(320, 568), textScale: 1.3);
    await open(tester, S.settings);
    expect(tester.takeException(), isNull);
    await open(tester, S.support);
    expect(tester.takeException(), isNull);
    expect(find.bySemanticsLabel(S.copyReport), findsOneWidget);
    expect(find.bySemanticsLabel(S.writeSupport), findsNothing);
  });

  testWidgets('the apps screen fits a 320 px phone with 130 % text', (tester) async {
    final scene = home(
      VpnState.idle,
      settings: {
        appsKey: {
          'mode': 'only',
          'list': ['org.example.game'],
        },
      },
      apps: FakeApps(lockdown: true),
    );
    await show(tester, scene, const Size(320, 568), textScale: 1.3);
    await open(tester, S.settings);
    expect(tester.takeException(), isNull);
    await open(tester, S.apps);
    expect(tester.takeException(), isNull);
    for (final mode in [S.appsAll, S.appsExcept, S.appsOnly]) {
      await tester.tap(find.bySemanticsLabel(mode));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      expect(tester.takeException(), isNull, reason: mode);
    }
    final hint = tester.renderObject<RenderParagraph>(find.text(S.appsHint(AppsMode.only)));
    expect(hint.didExceedMaxLines, isFalse);
    for (final segment in tester.widgetList<Pressable>(
      find.descendant(of: find.byType(Segments<AppsMode>), matching: find.byType(Pressable)),
    )) {
      expect(tester.getSize(find.byWidget(segment)).height, greaterThanOrEqualTo(44));
    }
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
    expect(S.expiry(subscription(days: 42), clock), 'Ещё 42 дня · до 31 декабря');
    expect(S.expiry(subscription(days: 60), clock), 'Ещё 60 дней · до 18 января 2027');
    expect(S.expiry(subscription(days: 3), clock), 'Ещё 3 дня');
    expect(S.expiry(subscription(days: 0), clock), 'Заканчивается сегодня');
    expect(S.expiry(subscription(days: -2), clock), 'Закончилась 17 ноября');
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
