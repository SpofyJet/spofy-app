import 'package:flutter/foundation.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/client.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/state/model.dart';
import 'package:spofy/ui/app.dart';
import 'package:spofy/ui/home.dart';
import 'package:spofy/ui/icons.dart';
import 'package:spofy/ui/settings.dart';
import 'package:spofy/ui/strings.dart';
import 'package:spofy/ui/theme.dart';
import 'package:spofy/ui/widgets.dart';

import 'scenes.dart';

const offer = Update(version: '0.2.0', build: 2, notes: 'Быстрее подключается', size: 31457280);

Brief brief(String id, {String title = '', String kind = 'url', bool active = false, String stop = ''}) => Brief(
  id: id,
  title: title,
  kind: kind,
  active: active,
  servers: 11,
  used: 13312 * 1024 * 1024,
  total: 102400 * 1024 * 1024,
  expire: clock.add(const Duration(days: 42, hours: 5)).millisecondsSinceEpoch ~/ 1000,
  stop: stop,
);

Future<void> box(WidgetTester tester, Widget child, {double width = 360, double textScale = 1}) async {
  tester.view.devicePixelRatio = 1;
  tester.view.physicalSize = const Size(400, 400);
  tester.platformDispatcher.textScaleFactorTestValue = textScale;
  addTearDown(tester.view.reset);
  addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);
  await tester.pumpWidget(
    MediaQuery(
      data: MediaQueryData(size: const Size(400, 400), textScaler: TextScaler.linear(textScale)),
      child: Directionality(
        textDirection: TextDirection.ltr,
        child: DefaultTextStyle(
          style: Ink.body,
          child: Align(
            alignment: Alignment.topLeft,
            child: SizedBox(width: width, child: child),
          ),
        ),
      ),
    ),
  );
}

void main() {
  setUpAll(loadAssets);
  setUp(() => S.use(Lang.ru));

  testWidgets('notices on home sit above the tiles and never move the power button', (tester) async {
    await show(tester, home(VpnState.idle), phone);
    final calm = tester.getCenter(find.byType(PowerControl));
    final offered = home(VpnState.idle, update: offer)..model.failure = const Failure(Task.disconnect, 'busy', '');
    await show(tester, offered, phone);
    expect(find.text(S.updateBanner(offer)), findsOneWidget);
    expect(find.text(S.failure(offered.model.failure!)), findsOneWidget);
    expect(tester.getCenter(find.byType(PowerControl)), calm);
    for (final size in [phone, small]) {
      await show(tester, home(VpnState.connected, server: 'nl', delay: 38), size, textScale: 1.3);
      final gap = tester.getRect(find.byType(Tile).first).top - tester.getRect(find.byType(Speed)).bottom;
      expect(gap, greaterThanOrEqualTo(16));
    }
  });

  testWidgets('home shows only its own failures, not those left by other screens', (tester) async {
    for (final task in [Task.measure, Task.checkUpdate, Task.refresh, Task.subscribe]) {
      final scene = home(VpnState.idle)..model.failure = Failure(task, 'sub_unreachable', '');
      await show(tester, scene, phone);
      expect(find.text(S.failure(scene.model.failure!)), findsNothing, reason: task.name);
    }
  });

  testWidgets('the timer dot tells a working tunnel from a stalled one', (tester) async {
    Color dot() => tester
        .widgetList<DecoratedBox>(find.descendant(of: find.byType(Uptime), matching: find.byType(DecoratedBox)))
        .map((b) => (b.decoration as BoxDecoration).color)
        .whereType<Color>()
        .last;
    await show(tester, home(VpnState.connected, server: 'nl'), phone);
    expect(dot(), Ink.good);
    await show(tester, home(VpnState.stalled, server: 'nl'), phone);
    expect(dot(), Ink.warning);
    await show(tester, home(VpnState.offline, server: 'nl'), phone);
    expect(dot(), Ink.warning);
  });

  testWidgets('home names a server the way the server list does', (tester) async {
    const raw = Server(
      id: 'de',
      name: '(DE)',
      label: '(DE)',
      country: 'DE',
      protocol: 'vless',
      tier: 0,
      delayMs: 44,
      measuredAt: 0,
    );
    final scene = Scene(
      connection: const Connection(state: VpnState.idle, choice: 'de'),
      subscription: subscription(),
      servers: const [raw],
    );
    await show(tester, scene, phone);
    expect(find.text('Германия'), findsOneWidget);
    expect(find.text('(DE)'), findsNothing);
  });

  testWidgets('when the core is lost the person can try again without restarting the app', (tester) async {
    var attempts = 0;
    final model = AppModel.opening(() async {
      attempts++;
      if (attempts == 1) {
        throw const CoreClosed();
      }
      return CoreClient(SilentTransport());
    });
    system(tester);
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = phone;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(SpofyApp(model: model, now: () => clock));
    await tester.pump();
    expect(model.fault, AppModel.coreClosed);
    expect(find.text(S.coreClosed), findsOneWidget);
    await tester.tap(find.bySemanticsLabel(S.tryAgain));
    await tester.pump();
    await tester.pump();
    expect(attempts, 2);
    expect(model.fault, '');
    expect(find.text(S.coreClosed), findsNothing);
    expect(find.bySemanticsLabel(S.tryAgain), findsNothing);
  });

  for (final theme in ['dark', 'light']) {
    testWidgets('a subscription without servers says so and offers to refresh it, $theme', (tester) async {
      final scene = Scene(subscription: subscription(), settings: {themeKey: theme});
      await show(tester, scene, phone);
      await open(tester, S.server);
      expect(find.text(S.noServers), findsOneWidget);
      expect(find.text(S.auto), findsNothing);
      expect(tester.widget<RoundButton>(find.byType(RoundButton).last).onTap, isNull);
      await golden(tester, theme == 'dark' ? 'servers_none_360' : 'servers_none_light_360');
      await tester.tap(find.bySemanticsLabel(S.refreshSubscription));
      await tester.pump();
      expect(scene.transport.sent.last, contains('"op":"refresh"'));
    });
  }

  testWidgets('the measure button shows a gauge, not the reload arrow', (tester) async {
    await show(tester, home(VpnState.idle), phone);
    await open(tester, S.server);
    expect(tester.widget<RoundButton>(find.byType(RoundButton).last).glyph, Glyph.speed);
  });

  testWidgets('on a small phone with large text names are whole and the delay moves under the protocol', (
    tester,
  ) async {
    await show(tester, home(VpnState.idle), const Size(320, 568), textScale: 1.3);
    await open(tester, S.server);
    for (final name in ['Нидерланды', 'Германия']) {
      expect(tester.renderObject<RenderParagraph>(find.text(name)).didExceedMaxLines, isFalse, reason: name);
    }
    expect(tester.getTopLeft(find.text('44 мс')).dy, greaterThan(tester.getBottomLeft(find.text('Германия')).dy));
    expect(tester.getTopLeft(find.text('44 мс')).dx, tester.getTopLeft(find.text('Германия')).dx);
  });

  testWidgets('a folded line breaks between parts and never leaves the dot at the end of a line', (tester) async {
    await box(tester, const Folded(['Самый быстрый сервер', '38 мс']), width: 170);
    final first = tester.getTopLeft(find.text('Самый быстрый сервер'));
    final second = tester.getTopLeft(find.text('38 мс'));
    expect(second.dx, first.dx);
    expect(second.dy, greaterThan(first.dy));
    await box(tester, const Folded(['Нидерланды', '38 мс']), width: 300);
    expect(tester.getTopLeft(find.text('38 мс')).dy, tester.getTopLeft(find.text('Нидерланды')).dy);
  });

  testWidgets('balanced text keeps its line count and evens the lines out', (tester) async {
    const text = 'VPN выключен до продления. Оплата по СБП или картой';
    await box(tester, Balanced(text, style: Ink.note), width: 330);
    final paragraph = tester.renderObject<RenderParagraph>(find.text(text));
    final plain = TextPainter(
      text: TextSpan(text: text, style: Ink.note),
      textDirection: TextDirection.ltr,
    )..layout(maxWidth: 330);
    expect(paragraph.size.height, plain.height);
    expect(paragraph.size.width, lessThan(330));
    plain.dispose();
  });

  testWidgets('the active subscription in the list does not repeat the card; own keys show their servers', (
    tester,
  ) async {
    await show(
      tester,
      home(
        VpnState.idle,
        browse: (_) {},
        briefs: [
          brief('a', title: '🛡 Spofy VPN', active: true),
          brief('b'),
        ],
      ),
      phone,
    );
    await open(tester, S.subscription);
    final active = tester.widget<Tile>(find.widgetWithText(Tile, 'Spofy VPN'));
    expect(active.subtitle, S.serversCount(11));
    expect(find.textContaining('🛡'), findsNothing);
    await show(
      tester,
      home(
        VpnState.idle,
        plan: subscription(total: 0, used: 0, expire: 0),
        briefs: [brief('k', kind: 'links', active: true)],
      ),
      phone,
    );
    await open(tester, S.subscription);
    expect(find.text(S.serversCount(11)), findsOneWidget);
    expect(find.text(S.unlimited), findsNothing);
  });

  for (final theme in ['dark', 'light']) {
    testWidgets('another subscription is added in a form with its own cancel and no second filled button, $theme', (
      tester,
    ) async {
      await show(tester, home(VpnState.idle, browse: (_) {}, settings: {themeKey: theme}), phone);
      await open(tester, S.subscription);
      await open(tester, S.addAnother);
      expect(find.text(S.newSubscription.toUpperCase()), findsOneWidget);
      expect(find.byType(PrimaryButton), findsOneWidget);
      final field = tester.getSize(find.byType(EditableText));
      await tester.enterText(find.byType(EditableText), 'https://x');
      await tester.pump();
      expect(tester.getSize(find.byType(EditableText)).height, lessThanOrEqualTo(field.height));
      final panel = find.ancestor(of: find.byType(EditableText), matching: find.byType(Panel)).first;
      final height = tester.getSize(panel).height;
      await tester.enterText(find.byType(EditableText), '');
      await tester.pump();
      expect(tester.getSize(panel).height, height);
      await golden(tester, theme == 'dark' ? 'subscription_another_360' : 'subscription_another_light_360');
      await open(tester, S.cancel);
      expect(find.byType(EditableText), findsNothing);
      expect(find.bySemanticsLabel(S.addAnother), findsOneWidget);
    });
  }

  test('subscription titles lose the emoji around them', () {
    expect(S.briefTitle(brief('a', title: '🛡 Spofy VPN')), 'Spofy VPN');
    expect(S.briefTitle(brief('a', title: 'Работа 🔥')), 'Работа');
    expect(S.briefTitle(brief('a', title: '⭐')), S.subscription);
    expect(S.briefTitle(brief('a', title: 'VPN+')), 'VPN+');
  });

  testWidgets('a custom value field shows a short example, not the cut hint', (tester) async {
    await show(tester, home(VpnState.idle), phone);
    await open(tester, S.settings);
    await open(tester, S.advanced);
    await open(tester, S.dns);
    await open(tester, S.dnsOwn);
    expect(find.text(S.dnsExample), findsWidgets);
    expect(find.text(S.dnsOwnHint), findsOneWidget);
  });

  testWidgets('Escape and the mouse back button go back; Escape keeps a setting that waits for a reconnect', (
    tester,
  ) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.settings);
    expect(find.text(S.settings), findsWidgets);
    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text(S.connection.toUpperCase()), findsNothing);
    await open(tester, S.settings);
    final mouse = await tester.createGesture(kind: PointerDeviceKind.mouse, buttons: kBackMouseButton);
    await mouse.down(const Offset(200, 400));
    await mouse.up();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text(S.connection.toUpperCase()), findsNothing);
    final connected = home(VpnState.connected, server: 'nl');
    await show(tester, connected, phone);
    await connected.model.set(fragmentKey, true);
    await tester.pump();
    expect(find.text(S.reconnectAsk), findsOneWidget);
    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.pump();
    expect(connected.model.pending, isNull);
    expect(find.text(S.reconnectAsk), findsNothing);
  });

  testWidgets('on a computer the version and the report can be selected and copied', (tester) async {
    debugDefaultTargetPlatformOverride = TargetPlatform.windows;
    addTearDown(() => debugDefaultTargetPlatformOverride = null);
    await show(tester, home(VpnState.idle), phone);
    await open(tester, S.settings);
    await open(tester, S.version('0.1.0'));
    expect(
      find.ancestor(of: find.text(S.release('0.1.0', 1)), matching: find.byType(SelectableRegion)),
      findsOneWidget,
    );
    debugDefaultTargetPlatformOverride = null;
  });

  testWidgets('the report keeps room for the device lines while they load', (tester) async {
    final scene = home(VpnState.idle);
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await open(tester, S.support);
    expect(find.textContaining('Spofy 0.1.0\n\n\n'), findsOneWidget);
    scene.transport.answer(
      '{"re":1,"ok":true,"report":{"os":"Android","osVersion":"15","model":"Pixel 8","device":"hw-1","startedAt":0,"journal":[]}}',
    );
    await tester.pump();
    await tester.pump();
    expect(find.textContaining('Spofy 0.1.0\nAndroid 15 · Pixel 8\n'), findsOneWidget);
  });

  testWidgets('buttons wrap a long label instead of cutting it', (tester) async {
    await box(tester, PrimaryButton(label: S.manage, onTap: () {}), width: 240, textScale: 1.3);
    final text = tester.renderObject<RenderParagraph>(find.text(S.manage));
    expect(text.didExceedMaxLines, isFalse);
    expect(tester.getSize(find.byType(PrimaryButton)).height, greaterThanOrEqualTo(56));
    expect(tester.getTopLeft(find.text(S.manage)).dx, greaterThanOrEqualTo(16));
  });
}
