import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/ui/strings.dart';

import 'scenes.dart';

void main() {
  setUpAll(loadAssets);

  final screens = <String, (Scene Function(), List<String>)>{
    'home idle': (() => home(VpnState.idle), []),
    'home connected': (() => home(VpnState.connected, server: 'nl', delay: 38), []),
    'home failed': (() => home(VpnState.failed, code: 'no_server'), []),
    'home ended': (() => stopped('ended', days: -3), []),
    'servers': (() => home(VpnState.idle), [S.server]),
    'subscription': (() => home(VpnState.idle, browse: (_) {}), [S.subscription]),
    'settings': (() => home(VpnState.idle), [S.settings]),
    'apps': (() => home(VpnState.idle, apps: FakeApps()), [S.settings, S.apps]),
    'advanced': (() => home(VpnState.idle), [S.settings, S.advanced]),
    'dns': (() => home(VpnState.idle), [S.settings, S.advanced, S.dns]),
    'support': (
      () => home(VpnState.stalled, server: 'de', code: 'server_unreachable', browse: (_) {}),
      [S.settings, S.support],
    ),
    'subscription ending': (
      () => Scene(subscription: subscription(days: 2, used: 96000), servers: servers(), browse: (_) {}),
      [S.subscription],
    ),
  };

  for (final brightness in Brightness.values) {
    for (final entry in screens.entries) {
      testWidgets('${entry.key}, ${brightness.name}: tap targets, labels and text contrast', (tester) async {
        final semantics = tester.ensureSemantics();
        await show(tester, entry.value.$1(), phone, brightness: brightness);
        for (final label in entry.value.$2) {
          await open(tester, label);
        }
        await expectLater(tester, meetsGuideline(androidTapTargetGuideline));
        await expectLater(tester, meetsGuideline(iOSTapTargetGuideline));
        await expectLater(tester, meetsGuideline(labeledTapTargetGuideline));
        await expectLater(tester, meetsGuideline(textContrastGuideline));
        semantics.dispose();
      });
    }
  }
}
