import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/state/model.dart';
import 'package:spofy/ui/home.dart';
import 'package:spofy/ui/servers.dart';
import 'package:spofy/ui/settings.dart';
import 'package:spofy/ui/strings.dart';
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
    ], sort: sortSpeed);
    expect(
      [for (final p in list) (p.name, p.servers.map((s) => s.id).join())],
      [
        ('Обход', 'fg'),
        ('Germany', 'ba'),
        ('Frankfurt', 'd'),
        ('Германия', 'c'),
        ('Мой сервер', 'h'),
        ('🇳🇱 Netherlands', 'e'),
      ],
    );
    expect(places(servers(), sort: sortName).map((p) => p.name), [
      'Беларусь',
      'Германия',
      'Ирландия',
      'Литва',
      'Нидерланды',
      'Обход',
      'Польша',
      'Сингапур',
      'США',
      'Финляндия',
    ]);
    expect(places(servers(), sort: sortSpeed, query: 'hyster').map((p) => p.name), ['Польша']);
    expect(places(servers(), sort: sortSpeed, query: 'финл').map((p) => p.name), ['Финляндия']);
    expect(places(servers(), sort: sortSpeed, query: 'ru').map((p) => p.name), ['Обход']);
  });

  testWidgets('a place with several servers opens to show them, each with its protocol, and one is chosen', (
    tester,
  ) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.server);
    expect(find.text('VLESS · Reality'), findsWidgets);
    expect(find.text('Обход 1'), findsNothing);
    await tester.tap(find.text('Обход'));
    await tester.pump(const Duration(milliseconds: 300));
    expect(rows(tester).take(4), ['Обход', 'Обход 1', 'Обход 2', 'Нидерланды']);
    await golden(tester, 'servers_open_360');
    await tester.tap(find.text('Обход 2'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(scene.transport.sent.single, '{"id":1,"op":"connect","server":"bypass2"}');
  });

  test('a server that did not answer is told apart from one not yet checked and goes last', () {
    final list = servers();
    final sg = list.firstWhere((s) => s.id == 'sg');
    final unchecked = list.firstWhere((s) => s.id == 'bypass2');
    expect([sg.silent, sg.measured, S.answer(sg)], [true, false, S.silent]);
    expect([unchecked.silent, S.answer(unchecked)], [false, S.noDelay]);
    expect(ordered(list).last.id, 'sg');
    expect(places(list, sort: sortSpeed).last.name, 'Сингапур');
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

  testWidgets('search narrows the places, the order can be by name and is kept by the core', (tester) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.server);
    await tester.enterText(find.byType(EditableText), 'герм');
    await tester.pump();
    expect(rows(tester), ['Германия']);
    expect(find.text(S.auto), findsNothing);
    await tester.enterText(find.byType(EditableText), 'нет такой');
    await tester.pump();
    expect(find.text(S.nothingFound), findsOneWidget);
    await tester.enterText(find.byType(EditableText), '');
    await tester.pump();
    await tester.tap(find.text(S.sortName));
    await tester.pump();
    expect(scene.transport.sent.single, '{"id":1,"op":"set","key":"serverSort","value":"name"}');

    final named = home(VpnState.idle, settings: {serverSortKey: sortName});
    await show(tester, named, phone);
    await open(tester, S.server);
    expect(rows(tester).take(3), ['Беларусь', 'Германия', 'Ирландия']);
    await golden(tester, 'servers_by_name_360');
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

  testWidgets('tunnel settings are sent to the core, and a value it refuses is said so', (tester) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await scrollTo(tester, find.text(S.fragmentHint));
    await golden(tester, 'settings_tunnel_360');
    await press(tester, find.text(S.fragment));
    expect(scene.transport.sent.last, '{"id":1,"op":"set","key":"fragment","value":true}');
    await open(tester, S.dns);
    await tester.tap(find.text('Cloudflare'));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":2,"op":"set","key":"dns","value":"cloudflare"}');
    await tester.tap(find.text(S.dnsOwn));
    await tester.pump();
    await tester.enterText(find.byType(EditableText), ' 10.0.0.1 ');
    await tester.tap(find.bySemanticsLabel(S.save));
    await tester.pump();
    expect(scene.transport.sent.last, '{"id":3,"op":"set","key":"dns","value":"10.0.0.1"}');
    scene.transport.answer('{"re":3,"ok":false,"code":"bad_value","error":"bad setting"}');
    await tester.pump();
    await tester.pump();
    expect(find.text(S.badValue), findsOneWidget);
    await golden(tester, 'settings_dns_own_360');
  });

  testWidgets('how servers are checked is chosen in settings and sent to the core', (tester) async {
    final scene = home(VpnState.idle, settings: {probeTimeoutKey: 6});
    await show(tester, scene, phone);
    await open(tester, S.settings);
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
    expect(find.text('Pixel 8 · Android 15'), findsOneWidget);
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
