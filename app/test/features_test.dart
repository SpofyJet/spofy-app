import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/state/model.dart';
import 'package:spofy/ui/home.dart';
import 'package:spofy/ui/servers.dart';
import 'package:spofy/ui/settings.dart';
import 'package:spofy/ui/app.dart';
import 'package:spofy/ui/strings.dart';
import 'package:spofy/ui/theme.dart';
import 'package:spofy/ui/update.dart';

import 'scenes.dart';

const offer = Update(
  version: '0.2.0',
  build: 2,
  notes: 'Быстрее подключается\nТаймер на главном экране',
  size: 31457280,
);

List<String> copies(WidgetTester tester) {
  final copied = <String>[];
  tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
    if (call.method == 'Clipboard.setData') {
      copied.add((call.arguments as Map<Object?, Object?>)['text']! as String);
    }
    return null;
  });
  addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, null));
  return copied;
}

Future<void> press(WidgetTester tester, Finder target) async {
  await tester.ensureVisible(target);
  await tester.pump(const Duration(milliseconds: 300));
  await tester.tap(target);
  await tester.pump();
}

Future<void> scrollTo(WidgetTester tester, Finder target) async {
  await tester.scrollUntilVisible(target, 200, scrollable: find.byType(Scrollable).last);
  await tester.pump(const Duration(milliseconds: 300));
}

void main() {
  test('the time of the last update is told in plain words', () {
    final now = DateTime.utc(2026, 11, 21, 12);
    int ago(Duration d) => now.subtract(d).millisecondsSinceEpoch ~/ 1000;
    expect(S.fetched(0, now), '');
    expect(S.fetched(ago(const Duration(seconds: 20)), now), 'Обновлено только что');
    expect(S.fetched(ago(const Duration(minutes: 59)), now), 'Обновлено 59 мин назад');
    expect(S.fetched(ago(const Duration(hours: 2)), now), 'Обновлено 2 ч назад');
    expect(S.fetched(ago(const Duration(days: 3)), now), 'Обновлено 3 дня назад');
    expect(S.fetched(ago(const Duration(days: 5)), now), 'Обновлено 5 дней назад');
  });

  setUpAll(loadAssets);

  test('each server says how it connects, in the words its makers use', () {
    final named = {for (final s in servers()) s.id: S.via(s)};
    expect(named, {
      'nl': 'VLESS · Reality',
      'de': 'VLESS · XHTTP · Reality',
      'fi': 'Trojan · WebSocket · TLS',
      'pl': 'Hysteria 2',
      'lt': 'VLESS · gRPC · TLS',
      'ie': 'VLESS · Reality',
      'by': 'VMess · WebSocket · TLS',
      'us': 'Shadowsocks',
      'sg': 'VLESS · XHTTP · TLS',
      'bypass1': 'VLESS · XHTTP · TLS',
      'bypass2': 'VLESS · XHTTP · TLS',
    });
  });

  test('servers of one country and one name make one place; other names stay apart', () {
    Server at(String id, String label, String country, int delay) => Server(
      id: id,
      name: label,
      label: label,
      country: country,
      protocol: 'vless',
      tier: 0,
      delayMs: delay,
      measuredAt: delay > 0 ? 1 : 0,
    );
    final list = places([
      at('a', 'Germany 1', 'DE', 90),
      at('b', 'Germany 2', 'DE', 40),
      at('c', 'DE-3', 'DE', 0),
      at('d', 'Frankfurt', 'DE', 70),
      at('e', '🇳🇱 Netherlands (NL)', 'NL', 0),
      at('f', 'Обход #1', 'RU', 30),
      at('g', 'Обход #2', 'RU', 35),
      at('h', 'Мой сервер', '', 0),
    ]);
    expect(
      [for (final p in list) (p.name, p.servers.map((s) => s.id).join())],
      [
        ('Germany', 'ab'),
        ('Германия', 'c'),
        ('Frankfurt', 'd'),
        ('🇳🇱 Netherlands', 'e'),
        ('Обход', 'fg'),
        ('Мой сервер', 'h'),
      ],
    );
    expect(places(servers(), query: 'hyster').map((p) => p.name), ['Польша']);
    expect(places(servers(), query: 'финл').map((p) => p.name), ['Финляндия']);
    expect(places(servers(), query: 'ru').map((p) => p.name), ['Обход']);
    expect(places(servers()).map((p) => p.name), {for (final s in servers()) placeName(s)}.toList());
  });

  testWidgets('a place with several servers opens to show them, each with its protocol, and one is chosen', (
    tester,
  ) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.server);
    expect(find.text('VLESS · Reality'), findsWidgets);
    expect(find.text('Обход 1'), findsNothing);
    await open(tester, 'Обход');
    await tester.scrollUntilVisible(
      find.text('Обход 2'),
      200,
      scrollable: find.byWidgetPredicate((w) => w is Scrollable && w.axisDirection == AxisDirection.down).last,
    );
    await tester.pump(const Duration(milliseconds: 300));
    expect(rows(tester).skipWhile((r) => r != 'Обход').take(3), ['Обход', 'Обход 1', 'Обход 2']);
    await golden(tester, 'servers_open_360');
    await tester.tap(find.text('Обход 2'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(scene.transport.sent.single, '{"id":1,"op":"connect","server":"bypass2"}');
  });

  test('a server that did not answer is told apart from one not yet checked and keeps its place', () {
    final list = servers();
    final sg = list.firstWhere((s) => s.id == 'sg');
    final unchecked = list.firstWhere((s) => s.id == 'bypass2');
    expect([sg.silent, sg.measured, S.answer(sg)], [true, false, S.silent]);
    expect([unchecked.silent, S.answer(unchecked)], [false, S.noDelay]);
    expect(places(list)[list.indexOf(sg)].servers.single.id, 'sg');
  });

  testWidgets('with no subscription the empty screen leads to the shop, and after a failure to help', (tester) async {
    final opened = <String>[];
    final scene = Scene(subscription: null, browse: opened.add);
    await show(tester, scene, phone);
    await press(tester, find.bySemanticsLabel('${S.buyAsk} ${S.buy}'));
    expect(opened, [shopAddress]);
    await show(
      tester,
      Scene(subscription: null, browse: opened.add, failure: const Failure(Task.subscribe, 'sub_unreachable', '')),
      phone,
    );
    expect(find.bySemanticsLabel('${S.buyAsk} ${S.buy}'), findsNothing);
    await press(tester, find.bySemanticsLabel('${S.stuckAsk} ${S.stuck}'));
    expect(opened, [shopAddress, shopAddress]);
    await golden(tester, 'empty_stuck_360');
  });

  testWidgets('search narrows the places, and the list keeps the order of the subscription', (tester) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.server);
    expect(rows(tester), places(servers()).map((p) => p.name).take(rows(tester).length).toList());
    await tester.enterText(find.byType(EditableText), 'герм');
    await tester.pump();
    expect(rows(tester), ['Германия']);
    expect(find.text(S.auto), findsNothing);
    await tester.enterText(find.byType(EditableText), 'нет такой');
    await tester.pump();
    expect(find.text(S.nothingFound), findsOneWidget);
    expect(scene.transport.sent, isEmpty);
  });

  testWidgets('a setting that reconnects the VPN asks first while it protects and goes at once while it is off', (
    tester,
  ) async {
    Finder ask(String label) => find.descendant(of: find.byType(ReconnectAsk), matching: find.text(label));
    List<String> sets(Scene scene) => [
      for (final m in scene.transport.sent)
        if (m.contains('"op":"set"')) m.replaceFirst(RegExp(r'"id":\d+,'), ''),
    ];
    const detailed = '{"op":"set","key":"logLevel","value":"detailed"}';
    final scene = home(VpnState.connected, server: 'nl', delay: 38);
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await open(tester, S.logsDetailed);
    expect(sets(scene), isEmpty);
    expect(find.text(S.reconnectAsk), findsOneWidget);
    await golden(tester, 'settings_reconnect_360');
    await tester.tap(ask(S.cancel));
    await tester.pump();
    expect([find.text(S.reconnectAsk).evaluate().length, sets(scene).length], [0, 0]);
    await open(tester, S.logsDetailed);
    await tester.tap(ask(S.apply));
    await tester.pump();
    expect(sets(scene), [detailed]);
    expect(find.text(S.reconnectAsk), findsNothing);

    final idle = home(VpnState.idle);
    await show(tester, idle, phone);
    await open(tester, S.settings);
    await open(tester, S.logsDetailed);
    expect(sets(idle), [detailed]);
    expect(find.text(S.reconnectAsk), findsNothing);
  });

  testWidgets('with reduced motion a screen opens without sliding or fading', (tester) async {
    tester.platformDispatcher.accessibilityFeaturesTestValue = const FakeAccessibilityFeatures(disableAnimations: true);
    addTearDown(tester.platformDispatcher.clearAccessibilityFeaturesTestValue);
    await show(tester, home(VpnState.idle), phone);
    await tester.tap(find.bySemanticsLabel(RegExp('^${S.settings}')));
    await tester.pump();
    await tester.pump();
    final title = find.text(S.settings).last;
    expect(find.ancestor(of: title, matching: find.byType(SlideTransition)), findsNothing);
    expect(find.ancestor(of: title, matching: find.byType(FadeTransition)), findsNothing);
  });

  testWidgets('while the VPN protects, home shows for how long', (tester) async {
    await show(tester, home(VpnState.connected, server: 'nl', delay: 38), phone);
    expect(find.bySemanticsLabel('${S.uptimeLabel} 01:02:05'), findsOneWidget);
    expect(find.byType(Uptime), findsOneWidget);
    await show(tester, home(VpnState.idle), phone);
    expect(find.byType(Uptime), findsNothing);
    expect(S.uptime(const Duration(days: 2, hours: 3, minutes: 4, seconds: 5)), '2 д 03:04:05');
    expect(S.uptime(const Duration(seconds: -4)), '00:00:00');
  });

  testWidgets('tunnel settings are one level down, sent to the core, and a value it refuses is said so', (
    tester,
  ) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.settings);
    expect(find.text(S.fragment), findsNothing);
    expect(find.text('Фрагментация, DNS, IPv6, домашняя сеть'), findsOneWidget);
    await open(tester, S.advanced);
    await golden(tester, 'settings_tunnel_360');
    await press(tester, find.text(S.fragment));
    expect(scene.transport.sent.last, '{"id":1,"op":"set","key":"fragment","value":true}');
    await open(tester, S.ipv6);
    expect(find.text(S.ipv6Hint), findsOneWidget);
    await golden(tester, 'settings_ipv6_360');
    await tester.tap(find.text(S.ipv6Off));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":2,"op":"set","key":"ipv6","value":false}');
    await tester.tap(find.text(S.ipv6Auto));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":3,"op":"set","key":"ipv6","value":null}');
    await open(tester, S.back);
    await open(tester, S.dns);
    await tester.tap(find.text('Cloudflare'));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":4,"op":"set","key":"dns","value":"cloudflare"}');
    await tester.tap(find.text(S.dnsOwn));
    await tester.pump();
    await tester.enterText(find.byType(EditableText), ' 10.0.0.1 ');
    await tester.tap(find.bySemanticsLabel(S.save));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":5,"op":"set","key":"dns","value":"10.0.0.1"}');
    scene.transport.answer('{"re":5,"ok":false,"code":"bad_value","error":"bad setting"}');
    await tester.pump();
    await tester.pump();
    expect(find.text(S.badValue), findsOneWidget);
    await golden(tester, 'settings_dns_own_360');
  });

  testWidgets('IPv6 is Auto unless it was turned off, and the first screen says what was changed', (tester) async {
    expect(chosen(ipv6Choices, null), S.ipv6Auto);
    expect(chosen(ipv6Choices, true), S.ipv6Auto);
    expect(chosen(ipv6Choices, false), S.ipv6Off);
    final scene = home(VpnState.idle, settings: {themeKey: 'light', ipv6Key: false, lanKey: true});
    await show(tester, scene, phone);
    await open(tester, S.settings);
    expect(find.text('Изменено: IPv6, домашняя сеть'), findsOneWidget);
    await open(tester, S.advanced);
    expect(find.text(S.ipv6Off), findsOneWidget);
    await golden(tester, 'settings_tunnel_light_360');
    await open(tester, S.ipv6);
    await golden(tester, 'settings_ipv6_light_360');
    await tester.tap(find.text(S.ipv6Off));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":1,"op":"set","key":"ipv6","value":false}');
    addTearDown(() => S.use(Lang.ru));
    S.use(Lang.en);
    expect(
      S.advancedChanged([S.fragment, S.dns, S.ipv6, S.lan, S.checking]),
      'Changed: fragmentation, DNS, IPv6, local network, server check',
    );
  });

  testWidgets('how servers are checked is chosen in settings and sent to the core', (tester) async {
    final scene = home(VpnState.idle, settings: {probeTimeoutKey: 6});
    await show(tester, scene, phone);
    await open(tester, S.settings);
    expect(find.text('Изменено: проверка серверов'), findsOneWidget);
    await open(tester, S.advanced);
    await scrollTo(tester, find.text(S.checking.toUpperCase()));
    await golden(tester, 'settings_checking_360');
    await tester.tap(find.text(S.seconds(4)));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":1,"op":"set","key":"probeTimeout","value":null}');
    await open(tester, S.probeTarget);
    await tester.tap(find.text('Cloudflare'));
    await tester.pump();
    expect(
      scene.transport.sent.last,
      '{"id":2,"op":"set","key":"probeUrl","value":"http://cp.cloudflare.com/generate_204"}',
    );
  });

  testWidgets('the subscription reminder is chosen in settings: three days unless changed, or off', (tester) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await scrollTo(tester, find.text(S.remind));
    expect(find.text(S.remindBefore(3)), findsOneWidget);
    await open(tester, S.remind);
    expect(find.text(S.remindHint), findsOneWidget);
    await tester.tap(find.text(S.remindOff));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":1,"op":"set","key":"remind","value":0}');
    await tester.tap(find.text(S.remindBefore(7)));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":2,"op":"set","key":"remind","value":7}');
    await tester.tap(find.text(S.remindBefore(3)));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":3,"op":"set","key":"remind","value":null}');
    expect(S.remindBefore(1), 'За день');
    expect(S.remindBefore(5), 'За 5 дней');
  });

  testWidgets('the log is read from the core, copied whole and cleared', (tester) async {
    final copied = copies(tester);
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await scrollTo(tester, find.text(S.logsHint));
    await tester.tap(find.text(S.logsHint));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(scene.transport.sent.last, '{"id":1,"op":"logs"}');
    final at = clock.millisecondsSinceEpoch ~/ 1000;
    scene.transport.answer(
      '{"re":1,"ok":true,"logs":[{"at":${at - 60},"source":"app","text":"connected to nl in 412 ms"},'
      '{"at":${at - 30},"source":"engine","text":"[Warning] a stream ended early"}]}',
    );
    await tester.pump();
    await tester.pump();
    await golden(tester, 'logs_360');
    await tester.tap(find.bySemanticsLabel(S.copy));
    await tester.pump();
    expect(copied.single, startsWith('Spofy 0.1.0\n'));
    expect(copied.single, contains('app  connected to nl in 412 ms'));
    expect(copied.single, contains('engine  [Warning] a stream ended early'));
    await tester.tap(find.bySemanticsLabel(S.clear));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":2,"op":"clearLogs"}');
    await tester.pump(const Duration(seconds: 3));
  });

  testWidgets('about tells the version, build, engine and device and checks for updates on request', (tester) async {
    final scene = home(VpnState.idle, browse: (_) {});
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await scrollTo(tester, find.text(S.version('0.1.0')));
    await tester.tap(find.text(S.version('0.1.0')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(scene.transport.sent.last, '{"id":1,"op":"report"}');
    scene.transport.answer(
      '{"re":1,"ok":true,"report":{"os":"Android","osVersion":"15","model":"Pixel 8","device":"hw-1","startedAt":0,"journal":[]}}',
    );
    await tester.pump();
    await tester.pump();
    expect(find.text(S.release('0.1.0', 1)), findsOneWidget);
    expect(find.text('xray-406c64d'), findsOneWidget);
    expect([find.text('Pixel 8'), find.text('Android 15')], everyElement(findsOneWidget));
    await tester.tap(find.bySemanticsLabel(S.updateCheck));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":2,"op":"checkUpdate"}');
    scene.transport.answer('{"re":2,"ok":false,"code":"update_none","error":"this is the latest build"}');
    await tester.pump();
    await tester.pump();
    expect(find.text(S.updateLatest), findsOneWidget);
    await golden(tester, 'about_360');
  });

  testWidgets('a new version is offered once when the app opens; the person updates in one tap', (tester) async {
    final installed = <String>[];
    final scene = home(
      VpnState.idle,
      update: offer,
      install: (file) async {
        installed.add(file);
        return true;
      },
    );
    await show(tester, scene, phone);
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text(S.updateTitle(offer)), findsOneWidget);
    expect(find.text(S.updateLater), findsOneWidget);
    expect(scene.transport.sent.first, '{"id":1,"op":"set","key":"updateShown","value":2}');
    await golden(tester, 'update_360');
    await tester.tap(find.bySemanticsLabel(S.updateNow));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":2,"op":"fetchUpdate"}');
    scene.transport.answer('{"ev":"update","update":${_json(offer, done: 12582912)}}');
    await tester.pump();
    await golden(tester, 'update_fetching_360');
    scene.transport.answer(
      '{"ev":"update","update":${_json(offer, done: offer.size, file: '/state/updates/Spofy-0.2.0.apk')}}',
    );
    scene.transport.answer('{"re":2,"ok":true}');
    await tester.pump();
    await tester.pump();
    expect(installed, ['/state/updates/Spofy-0.2.0.apk']);
    expect(find.text(S.updateInstall), findsOneWidget);
  });

  testWidgets('an offer seen before waits on home; a required one cannot be put off', (tester) async {
    final seen = home(VpnState.idle, update: offer, settings: {updateShownKey: 2});
    await show(tester, seen, phone);
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text(S.updateTitle(offer)), findsNothing);
    expect(find.text(S.updateBanner(offer)), findsOneWidget);
    await golden(tester, 'home_update_360');
    await tester.tap(find.text(S.updateBanner(offer)));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text(S.updateTitle(offer)), findsOneWidget);

    const required = Update(version: '0.2.0', build: 2, required: true, size: 31457280);
    final must = home(VpnState.idle, update: required, settings: {updateShownKey: 2});
    await show(tester, must, phone);
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text(S.updateRequired), findsOneWidget);
    expect(find.text(S.updateLater), findsNothing);
    expect(must.transport.sent, isEmpty);
    await golden(tester, 'update_required_360');
  });

  testWidgets('several subscriptions are listed, one tap switches, and one more can be added', (tester) async {
    final scene = home(
      VpnState.idle,
      briefs: [
        Brief(
          id: 'a',
          title: 'Spofy VPN',
          kind: 'url',
          active: true,
          servers: 11,
          used: 13958643712,
          total: 107374182400,
          expire: clock.add(const Duration(days: 42)).millisecondsSinceEpoch ~/ 1000,
        ),
        const Brief(id: 'b', title: '', kind: 'links', active: false, servers: 3, used: 0, total: 0, expire: 0),
      ],
    );
    await show(tester, scene, phone);
    await open(tester, S.subscription);
    expect(find.text(S.ownKeys), findsOneWidget);
    await golden(tester, 'subscriptions_360');
    await tester.tap(find.text(S.ownKeys));
    await tester.pump();
    expect(scene.transport.sent.single, '{"id":1,"op":"activate","subscription":"b"}');
    await scrollTo(tester, find.text(S.addAnother));
    await tester.tap(find.text(S.addAnother));
    await tester.pump();
    await tester.enterText(
      find.byType(EditableText),
      'vless://00000000-0000-0000-0000-000000000000@example.test:443#Мой',
    );
    await press(tester, find.bySemanticsLabel(S.add));
    expect(
      scene.transport.sent.last,
      '{"id":2,"op":"subscribe","url":"vless://00000000-0000-0000-0000-000000000000@example.test:443#Мой"}',
    );
  });
}

String _json(Update u, {int done = 0, String file = ''}) =>
    '{"version":"${u.version}","build":${u.build},"notes":${_quote(u.notes)},"required":${u.required},'
    '"size":${u.size},"done":$done,"file":"$file"}';

String _quote(String text) => '"${text.replaceAll('\n', r'\n')}"';
