import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/ui/strings.dart';
import 'package:spofy/ui/theme.dart';
import 'package:spofy/ui/widgets.dart';

import 'scenes.dart';

Set<Color> grounds(WidgetTester tester) {
  final found = <Color>{};
  for (final backdrop in find.byType(Backdrop, skipOffstage: false).evaluate()) {
    backdrop.visitChildren((child) => found.add((child.widget as ColoredBox).color));
  }
  return found;
}

List<String> sets(Scene scene) => [
  for (final line in scene.transport.sent)
    if (line.contains('"op":"set"')) line.substring(line.indexOf('"key"')),
];

Future<void> settle(WidgetTester tester) async {
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 400));
}

void main() {
  setUpAll(loadFonts);

  tearDown(() {
    S.use(Lang.ru);
    Ink.use(Ink.dark);
  });

  test('the theme is the chosen one, or the system one when nothing is chosen', () {
    expect(paletteFor(null, Brightness.dark), Ink.dark);
    expect(paletteFor(null, Brightness.light), Ink.light);
    expect(paletteFor('light', Brightness.dark), Ink.light);
    expect(paletteFor('dark', Brightness.light), Ink.dark);
    expect(paletteFor('sepia', Brightness.light), Ink.light);
    expect(Ink.light.bars.statusBarIconBrightness, Brightness.dark);
    expect(Ink.light.bars.systemNavigationBarIconBrightness, Brightness.dark);
    expect(Ink.dark.bars.statusBarIconBrightness, Brightness.light);
    expect(Ink.dark.bars.systemNavigationBarIconBrightness, Brightness.light);
  });

  test('the language is the chosen one, else the first system language the app can speak', () {
    for (final (setting, system, want) in [
      (null, const [Locale('ru', 'RU')], Lang.ru),
      (null, const [Locale('uk', 'UA'), Locale('en', 'US')], Lang.ru),
      (null, const [Locale('be')], Lang.ru),
      (null, const [Locale('kk', 'KZ')], Lang.ru),
      (null, const [Locale('de', 'DE'), Locale('ru', 'RU')], Lang.ru),
      (null, const [Locale('de', 'DE'), Locale('en', 'GB'), Locale('ru', 'RU')], Lang.en),
      (null, const [Locale('fr', 'FR')], Lang.en),
      (null, const <Locale>[], Lang.en),
      ('ru', const [Locale('en', 'US')], Lang.ru),
      ('en', const [Locale('ru', 'RU')], Lang.en),
      ('de', const [Locale('ru', 'RU')], Lang.ru),
    ]) {
      expect(langFor(setting, system), want, reason: '$setting with $system');
    }
  });

  test('numbers, counts and dates read naturally in English', () {
    S.use(Lang.en);
    expect(S.bytes(0), '0 B');
    expect(S.bytes(1536), '1.5 KB');
    expect(S.bytes(1288490188), '1.2 GB');
    expect(S.bytes(107374182400), '100 GB');
    expect([1, 2, 5, 11, 21, 111].map(S.days).toList(), [
      '1 day',
      '2 days',
      '5 days',
      '11 days',
      '21 days',
      '111 days',
    ]);
    expect([1, 2, 21].map(S.serversCount).toList(), ['1 server', '2 servers', '21 servers']);
    expect([1, 3].map(S.licenseCount).toList(), ['1 license', '3 licenses']);
    expect(S.until(subscription(days: 42), clock), 'until 31 Dec');
    expect(S.until(subscription(days: 60), clock), 'until 18 Jan 2027');
    expect(S.until(subscription(days: 3), clock), '3 days left');
    expect(S.until(subscription(days: 1), clock), '1 day left');
    expect(S.until(subscription(days: 0), clock), 'ends today');
    expect(S.until(subscription(days: -2), clock), 'ended');
    expect(S.usage(subscription(used: 96000)), '93.8 of 100 GB');
    expect(S.usage(subscription()), '13 of 100 GB');
    expect(S.usage(subscription(total: 0)), 'Unlimited');
    expect(S.expiry(subscription(days: 42), clock), '42 days left · until 31 December');
    expect(S.expiry(subscription(days: 60), clock), '60 days left · until 18 January 2027');
    expect(S.expiry(subscription(days: 3), clock), '3 days left');
    expect(S.expiry(subscription(days: 0), clock), 'Ends today');
    expect(S.expiry(subscription(days: -2), clock), 'Ended on 17 November');
    expect(S.uptime(const Duration(days: 2, hours: 3, minutes: 4, seconds: 5)), '2d 03:04:05');
    expect(S.delay(38), '38 ms');
    expect(S.seconds(4), '4 s');
    expect(S.country('de'), 'Germany');
    expect(S.reason('teleport'), 'Error: teleport');
    S.use(Lang.ru);
    expect(S.usage(subscription(used: 96000)), '93,8 из 100 ГБ');
    expect(S.days(21), '21 день');
  });

  for (final (name, scene, route) in [
    (
      'home_connected_light',
      () => home(VpnState.connected, server: 'nl', delay: 38, settings: {themeKey: 'light'}),
      '',
    ),
    ('servers_light', () => home(VpnState.idle, settings: {themeKey: 'light'}), 'server'),
    ('settings_light', () => home(VpnState.idle, settings: {themeKey: 'light'}), 'settings'),
    ('home_connected_en', () => home(VpnState.connected, server: 'nl', delay: 38, settings: {languageKey: 'en'}), ''),
    ('settings_en', () => home(VpnState.idle, settings: {languageKey: 'en'}), 'settings'),
  ]) {
    testWidgets(name, (tester) async {
      await show(tester, scene(), phone);
      if (route == 'server') {
        await open(tester, S.server);
      } else if (route == 'settings') {
        await open(tester, S.settings);
      }
      await golden(tester, '${name}_360');
    });
  }

  testWidgets('a chosen theme repaints every open screen and the system bars at once', (tester) async {
    final scene = home(VpnState.connected, server: 'nl', delay: 38);
    await show(tester, scene, phone);
    expect(grounds(tester), {Ink.dark.background});
    expect(SystemChrome.latestStyle?.statusBarIconBrightness, Brightness.light);
    await open(tester, S.settings);
    await open(tester, S.theme);
    expect(find.text(S.themeNow(true)), findsOneWidget);
    await open(tester, S.themeLight);
    expect(sets(scene), ['"key":"theme","value":"light"}']);
    scene.transport.answer('{"ev":"settings","settings":{"theme":"light"}}');
    await settle(tester);
    expect(grounds(tester), {Ink.light.background});
    expect(tester.widget<Text>(find.text(S.theme)).style?.color, Ink.light.text);
    expect(SystemChrome.latestStyle?.statusBarIconBrightness, Brightness.dark);
    expect(SystemChrome.latestStyle?.systemNavigationBarIconBrightness, Brightness.dark);
    await open(tester, S.themeSystem);
    scene.transport.answer('{"ev":"settings","settings":{}}');
    await settle(tester);
    expect(sets(scene).last, '"key":"theme","value":null}');
    expect(grounds(tester), {Ink.dark.background});
    expect(SystemChrome.latestStyle?.statusBarIconBrightness, Brightness.light);
  });

  testWidgets('without a chosen theme the app follows the system as it changes, with one it stays', (tester) async {
    await show(tester, home(VpnState.idle), phone);
    expect(grounds(tester), {Ink.dark.background});
    tester.platformDispatcher.platformBrightnessTestValue = Brightness.light;
    await settle(tester);
    expect(grounds(tester), {Ink.light.background});
    expect(SystemChrome.latestStyle?.statusBarIconBrightness, Brightness.dark);
    await open(tester, S.settings);
    await open(tester, S.theme);
    expect(find.text(S.themeNow(false)), findsOneWidget);

    await show(tester, home(VpnState.idle, settings: {themeKey: 'dark'}), phone, brightness: Brightness.light);
    expect(grounds(tester), {Ink.dark.background});
    tester.platformDispatcher.platformBrightnessTestValue = Brightness.dark;
    await settle(tester);
    tester.platformDispatcher.platformBrightnessTestValue = Brightness.light;
    await settle(tester);
    expect(grounds(tester), {Ink.dark.background});
    expect(SystemChrome.latestStyle?.statusBarIconBrightness, Brightness.light);
  });

  testWidgets('a chosen language rewrites every open screen at once', (tester) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, 'Настройки');
    await open(tester, 'Язык');
    expect(find.text('Как в системе: Русский'), findsOneWidget);
    await open(tester, 'English');
    expect(scene.transport.sent.single, '{"id":1,"op":"set","key":"language","value":"en"}');
    scene.transport.answer('{"ev":"settings","settings":{"language":"en"}}');
    await settle(tester);
    expect(find.text('Language'), findsOneWidget);
    expect(find.text('Follows the system: Русский'), findsOneWidget);
    expect(tester.widget<WidgetsApp>(find.byType(WidgetsApp)).locale, const Locale('en'));
    await open(tester, 'Back');
    expect(find.text('INTERFACE'), findsOneWidget);
    expect(find.text('English'), findsOneWidget);
    await open(tester, 'Back');
    expect(find.text('VPN is off'), findsOneWidget);
    expect(find.bySemanticsLabel(RegExp('^Turn VPN on')), findsOneWidget);
  });

  testWidgets('without a chosen language the app speaks the system one and follows it', (tester) async {
    await show(tester, home(VpnState.idle), phone, locales: const [Locale('de', 'DE'), Locale('uk', 'UA')]);
    expect(find.text('VPN выключен'), findsOneWidget);
    tester.platformDispatcher.localesTestValue = const [Locale('fr', 'FR')];
    await settle(tester);
    expect(find.text('VPN is off'), findsOneWidget);
    expect(tester.widget<WidgetsApp>(find.byType(WidgetsApp)).locale, const Locale('en'));

    await show(tester, home(VpnState.idle, settings: {languageKey: 'ru'}), phone, locales: const [Locale('en', 'US')]);
    expect(find.text('VPN выключен'), findsOneWidget);
    tester.platformDispatcher.localesTestValue = const [Locale('fr', 'FR')];
    await settle(tester);
    expect(find.text('VPN выключен'), findsOneWidget);
  });
}
