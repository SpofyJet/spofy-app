import 'package:flutter/rendering.dart';
import 'package:flutter/scheduler.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/ui/app.dart';
import 'package:spofy/ui/home.dart';
import 'package:spofy/ui/icons.dart';
import 'package:spofy/ui/motion.dart';
import 'package:spofy/ui/servers.dart';
import 'package:spofy/ui/strings.dart';
import 'package:spofy/ui/theme.dart';
import 'package:spofy/ui/widgets.dart';

import 'scenes.dart';

var _drawn = 0;

const _frame = Duration(microseconds: 16667);

Future<int> frames(WidgetTester tester, Duration span) async {
  final before = _drawn;
  for (var left = span; left > Duration.zero; left -= _frame) {
    await tester.pump(_frame);
  }
  return _drawn - before;
}

Future<void> showMoving(WidgetTester tester, Scene scene, {Map<String, Object?>? settings}) async {
  await show(tester, scene, phone);
  final start = tester.binding.clock.now();
  await tester.pumpWidget(
    SpofyApp(
      key: ObjectKey(scene),
      model: scene.model,
      now: () => clock.add(tester.binding.clock.now().difference(start)),
    ),
  );
  await tester.pump(const Duration(milliseconds: 300));
}

void still(WidgetTester tester) {
  tester.platformDispatcher.accessibilityFeaturesTestValue = const FakeAccessibilityFeatures(disableAnimations: true);
  addTearDown(tester.platformDispatcher.clearAccessibilityFeaturesTestValue);
}

void hide(WidgetTester tester) {
  tester.binding
    ..handleAppLifecycleStateChanged(AppLifecycleState.inactive)
    ..handleAppLifecycleStateChanged(AppLifecycleState.hidden);
  addTearDown(
    () => tester.binding
      ..handleAppLifecycleStateChanged(AppLifecycleState.inactive)
      ..handleAppLifecycleStateChanged(AppLifecycleState.resumed),
  );
}

void connect(Scene scene, VpnState state) {
  final c = scene.model.connection;
  scene.model
    ..snapshot = scene.model.snapshot.apply(
      ConnectionEvent(
        Connection(
          state: state,
          choice: c.choice,
          server: c.server,
          delayMs: c.delayMs,
          since: state.protecting ? clock.millisecondsSinceEpoch ~/ 1000 - 3725 : 0,
        ),
      ),
    )
    ..notifyListeners();
}

bool calm(BuildContext context) => MediaQuery.disableAnimationsOf(context);

void main() {
  setUpAll(() async {
    await loadAssets();
    SchedulerBinding.instance.addPersistentFrameCallback((_) => _drawn++);
  });

  tearDown(() {
    S.use(Lang.ru);
    Ink.use(Ink.dark);
  });

  testWidgets('a steady screen draws nothing; a connected one only ticks its clock', (tester) async {
    await show(tester, home(VpnState.idle), phone);
    expect(await frames(tester, const Duration(seconds: 10)), 0);
    await show(tester, home(VpnState.failed, code: 'no_server'), phone);
    expect(await frames(tester, const Duration(seconds: 10)), 0);

    await showMoving(tester, home(VpnState.connected, server: 'nl', delay: 38));
    final rebuilt = <Widget>[];
    final slot = tester.renderObject(
      find.ancestor(of: find.byType(Uptime), matching: find.byType(RepaintBoundary)).first,
    );
    bool inside(RenderObject r) {
      for (RenderObject? up = r; up != null; up = up.parent) {
        if (up == slot) {
          return true;
        }
      }
      return false;
    }

    var painted = 0;
    final outside = <String>{};
    debugOnRebuildDirtyWidget = (element, _) => rebuilt.add(element.widget);
    debugOnProfilePaint = (r) {
      painted++;
      if (!inside(r)) {
        outside.add('${r.runtimeType} ${r.paintBounds}');
      }
    };
    final drawn = await frames(tester, const Duration(seconds: 10));
    debugOnRebuildDirtyWidget = null;
    debugOnProfilePaint = null;
    printOnFailure('$drawn frames, ${rebuilt.length} builds, $painted paints');
    expect(drawn, inInclusiveRange(10, 10 * (2 + (Motion.large.inMicroseconds / _frame.inMicroseconds).ceil())));
    expect(rebuilt.whereType<HomeScreen>(), isEmpty);
    expect(rebuilt.whereType<PowerControl>(), isEmpty);
    expect(outside, isEmpty);
    expect(slot.paintBounds.size, const Size(320, Uptime.slot));
  });

  testWidgets('with motion off a connected screen draws once a second', (tester) async {
    await showMoving(tester, home(VpnState.connected, server: 'nl', delay: 38, settings: {motionKey: false}));
    expect(await frames(tester, const Duration(seconds: 10)), inInclusiveRange(10, 11));
  });

  testWidgets('while connecting only the ring turns: no widget is built, one painter draws', (tester) async {
    await show(tester, home(VpnState.connecting, server: 'nl'), phone);
    final rebuilt = <Widget>[];
    final painted = <RenderObject>[];
    debugOnRebuildDirtyWidget = (element, _) => rebuilt.add(element.widget);
    debugOnProfilePaint = painted.add;
    final drawn = await frames(tester, const Duration(seconds: 1));
    debugOnRebuildDirtyWidget = null;
    debugOnProfilePaint = null;
    expect(drawn, inInclusiveRange(59, 61));
    expect(rebuilt, isEmpty);
    expect(painted.length, drawn);
    expect(painted.map((r) => r.runtimeType).toSet(), {RenderCustomPaint});
    expect(painted.map((r) => r.paintBounds.size).toSet(), {const Size.square(PowerControl.size)});
  });

  testWidgets('the ring stops when the window is hidden, behind another screen and with motion off', (tester) async {
    final scene = home(VpnState.connecting, server: 'nl');
    await show(tester, scene, phone);
    hide(tester);
    await tester.pump();
    expect(await frames(tester, const Duration(seconds: 3)), 0);

    await show(tester, home(VpnState.connected, server: 'nl', delay: 38), phone);
    hide(tester);
    await tester.pump();
    expect(await frames(tester, const Duration(seconds: 3)), 0);

    final covered = home(VpnState.connecting, server: 'nl');
    await show(tester, covered, phone);
    await open(tester, S.settings);
    expect(await frames(tester, const Duration(seconds: 3)), 0);

    await show(tester, home(VpnState.connecting, server: 'nl', settings: {motionKey: false}), phone);
    expect(await frames(tester, const Duration(seconds: 3)), 0);
  });

  for (final theme in ['dark', 'light']) {
    testWidgets('the moment of connection ends exactly in the steady look, $theme', (tester) async {
      final scene = home(VpnState.connected, server: 'nl', delay: 38, settings: {themeKey: theme});
      final steady = scene.model.connection;
      connect(scene, VpnState.connecting);
      await show(tester, scene, phone);
      await tester.pump(const Duration(milliseconds: 700));
      scene.model
        ..snapshot = scene.model.snapshot.apply(ConnectionEvent(steady))
        ..notifyListeners();
      await tester.pump();
      await tester.pump(Motion.large);
      if (theme == 'dark') {
        await golden(tester, 'home_sealing_360');
      }
      final drawn = await frames(tester, Motion.settle);
      expect(drawn, inInclusiveRange(1, Motion.settle.inMicroseconds ~/ _frame.inMicroseconds + 1));
      await golden(tester, theme == 'dark' ? 'home_connected_360' : 'home_connected_light_360');
      expect(await frames(tester, const Duration(milliseconds: 500)), lessThanOrEqualTo(1));
    });
  }

  testWidgets('with motion off the state changes in one frame and screens open at once', (tester) async {
    final scene = home(VpnState.connecting, server: 'nl', delay: 38, settings: {motionKey: false});
    await show(tester, scene, phone);
    expect(calm(tester.element(find.byType(PowerControl))), isTrue);
    connect(scene, VpnState.connected);
    await tester.pump();
    await tester.pump();
    expect(await frames(tester, const Duration(seconds: 1)), lessThanOrEqualTo(1));
    final glyph = tester.widget<GlyphIcon>(
      find.descendant(of: find.byType(PowerControl), matching: find.byType(GlyphIcon)),
    );
    expect(glyph.color, Ink.onAccent);
    expect(find.text(S.title(VpnState.connecting)), findsNothing);

    await tester.tap(find.bySemanticsLabel(RegExp('^${S.settings}')));
    await tester.pump();
    expect(find.text(S.settings).hitTestable(), findsOneWidget);
    expect(find.byType(SlideTransition), findsNothing);
    expect(await frames(tester, const Duration(milliseconds: 300)), lessThanOrEqualTo(1));
  });

  testWidgets('a server group unfolds in 240 ms, or at once with motion off', (tester) async {
    for (final off in [false, true]) {
      await show(tester, home(VpnState.idle, settings: {if (off) motionKey: false}), phone);
      await open(tester, S.server);
      final group = find.byType(PlaceTile);
      await tester.scrollUntilVisible(
        group,
        200,
        scrollable: find.byWidgetPredicate((w) => w is Scrollable && w.axisDirection == AxisDirection.down),
      );
      await tester.pump(const Duration(milliseconds: 300));
      expect(find.byType(ServerTile), findsNWidgets(9));
      await tester.tap(group);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 120));
      final half = tester.getSize(find.byType(Unfold).last).height;
      expect(find.byType(ServerTile), findsNWidgets(11));
      await tester.pump(const Duration(milliseconds: 160));
      final full = tester.getSize(find.byType(Unfold).last).height;
      printOnFailure('off $off: $half of $full');
      expect(half, off ? full : allOf(greaterThan(0), lessThan(full)));
      await tester.tap(group);
      await tester.pump();
      expect(find.byType(ServerTile), findsNWidgets(off ? 9 : 11));
      await tester.pump(Motion.large);
      await tester.pump(_frame);
      expect(find.byType(ServerTile), findsNWidgets(9));
    }
  });

  testWidgets('«Анимации» is kept by the core, changes nothing in the tunnel and gives way to the system', (
    tester,
  ) async {
    final scene = home(VpnState.connected, server: 'nl', delay: 38);
    List<String> sets() => [
      for (final m in scene.transport.sent)
        if (m.contains('"op":"set"')) m.replaceFirst(RegExp(r'"id":\d+,'), ''),
    ];
    await show(tester, scene, phone);
    await open(tester, S.settings);
    await tester.ensureVisible(find.bySemanticsLabel(S.motion));
    await tester.pump(const Duration(milliseconds: 300));
    expect(calm(tester.element(find.byType(Header).last)), isFalse);
    await tester.tap(find.bySemanticsLabel(S.motion));
    await tester.pump();
    expect(sets(), ['{"op":"set","key":"motion","value":false}']);
    expect(find.text(S.reconnectAsk), findsNothing);
    scene.transport.answer('{"ev":"settings","settings":{"motion":false}}');
    await tester.pump();
    await tester.pump();
    expect(calm(tester.element(find.byType(Header).last)), isTrue);
    expect(tester.widget<SwitchTile>(find.widgetWithText(SwitchTile, S.motion)).value, isFalse);
    await tester.tap(find.bySemanticsLabel(S.motion));
    await tester.pump();
    expect(sets().last, '{"op":"set","key":"motion","value":null}');
    expect(scene.transport.sent.where((m) => m.contains('"connect"') || m.contains('"disconnect"')), isEmpty);
  });

  for (final theme in ['dark', 'light']) {
    testWidgets('the system reduced motion wins over the setting, $theme', (tester) async {
      still(tester);
      final scene = home(VpnState.connecting, server: 'nl', settings: {themeKey: theme});
      await show(tester, scene, phone);
      expect(await frames(tester, const Duration(seconds: 2)), 0);
      await open(tester, S.settings);
      await tester.scrollUntilVisible(find.text(S.motionSystem), 200, scrollable: find.byType(Scrollable).last);
      await tester.pump(const Duration(milliseconds: 300));
      expect(find.widgetWithText(SwitchTile, S.motion), findsNothing);
      expect(find.text(S.motionSystem), findsOneWidget);
      await golden(tester, theme == 'dark' ? 'settings_motion_system_360' : 'settings_motion_system_light_360');
    });
  }

  testWidgets('overscroll on Android stretches instead of a glow, and stays still with motion off', (tester) async {
    await show(tester, home(VpnState.idle), phone);
    await open(tester, S.settings);
    expect(find.byType(StretchingOverscrollIndicator), findsOneWidget);
    expect(find.byType(GlowingOverscrollIndicator), findsNothing);
    await show(tester, home(VpnState.idle, settings: {motionKey: false}), phone);
    await open(tester, S.settings);
    expect(find.byType(StretchingOverscrollIndicator), findsNothing);
    expect(find.byType(GlowingOverscrollIndicator), findsNothing);
  }, variant: TargetPlatformVariant.only(TargetPlatform.android));
}
