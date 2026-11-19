import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/client.dart';
import 'package:spofy/core/process_transport.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/state/model.dart';

Future<String> fetch(String url) async {
  final result = await Process.run('curl', ['-s', '-m', '5', url]);
  return (result.stdout as String).trim();
}

Future<void> until(AppModel model, bool Function() done) {
  if (done()) {
    return Future.value();
  }
  final waiting = Completer<void>();
  void look() {
    if (done() && !waiting.isCompleted) {
      model.removeListener(look);
      waiting.complete();
    }
  }

  model.addListener(look);
  return waiting.future.timeout(const Duration(seconds: 20));
}

void main() {
  test('the interface drives the real core through a real tunnel', () async {
    final env = Platform.environment;
    final servers = (jsonDecode(env['SPOFY_STAND_SERVERS']!) as Map<String, dynamic>).cast<String, String>();
    final client = env['SPOFY_STAND_CLIENT']!;
    final watch = Stopwatch()..start();
    final times = <String, int>{};
    Future<T> timed<T>(String name, Future<T> Function() action) async {
      final began = watch.elapsedMilliseconds;
      final result = await action();
      times[name] = watch.elapsedMilliseconds - began;
      return result;
    }

    Future<(ProcessTransport, AppModel, List<VpnState>)> launch() async {
      final transport = await ProcessTransport.start(env['SPOFY_CORE']!, [
        'serve',
        '-state',
        env['SPOFY_STAND_STATE']!,
        '-guard',
      ]);
      final model = AppModel(CoreClient(transport));
      final seen = <VpnState>[];
      var last = VpnState.idle;
      model.addListener(() {
        if (last != model.connection.state) {
          last = model.connection.state;
          seen.add(last);
        }
      });
      await model.start();
      expect(model.ready, isTrue, reason: transport.log.join('\n'));
      expect(model.fault, '');
      return (transport, model, seen);
    }

    var (transport, model, seen) = await timed('start', launch);
    expect(model.subscription, isNull);
    expect(await model.connect(), isFalse);
    expect(model.failure?.code, 'no_subscription');

    expect(
      await timed('subscribe', () => model.subscribe(env['SPOFY_STAND_URL']!)),
      isTrue,
      reason: '${model.failure}',
    );
    expect(model.subscription, isNotNull);
    expect(model.snapshot.servers.map((s) => s.id).toSet(), servers.keys.toSet());

    expect(await timed('measure', model.measure), isTrue);
    expect(model.snapshot.servers.every((s) => s.delayMs > 0), isTrue);

    seen.clear();
    expect(await timed('connect', model.connect), isTrue, reason: '${model.failure}');
    expect(seen, [VpnState.connecting, VpnState.connected]);
    final first = model.connection.server;
    expect(model.connection.auto, isTrue);
    expect(model.current?.id, first);
    expect(await fetch('http://foreign.test/ip'), servers[first]);
    expect(await fetch('http://local.test/ip'), client);
    await model.poll();
    expect(model.traffic.down, greaterThan(0));
    expect(model.traffic.up, greaterThan(0));
    expect(await model.measure(), isFalse);
    expect(model.failure?.code, 'connected');
    model.dismiss();

    final other = servers.keys.firstWhere((id) => id != first);
    seen.clear();
    expect(await timed('switch', () => model.choose(other)), isTrue, reason: '${model.failure}');
    expect(seen.last, VpnState.connected);
    expect(seen, isNot(contains(VpnState.idle)));
    expect(model.connection.server, other);
    expect(model.connection.choice, other);
    expect(await fetch('http://foreign.test/ip'), servers[other]);

    await model.check();
    expect(model.connection.state, VpnState.connected);
    expect(await model.refresh(), isTrue);
    expect(model.snapshot.servers, hasLength(servers.length));

    await model.set('autoConnect', true);
    expect(model.snapshot.settings['autoConnect'], isTrue);
    await transport.close();
    expect(await transport.exitCode, 0);

    (transport, model, seen) = await timed('restart', launch);
    expect(model.connection.choice, other);
    expect(model.subscription, isNotNull);
    expect(model.snapshot.servers.every((s) => s.delayMs > 0), isTrue);
    final restored = model;
    await timed('auto-connect', () => until(restored, () => restored.connection.state == VpnState.connected));
    expect(model.connection.server, other);
    await model.set('autoConnect', false);
    expect(await fetch('http://foreign.test/ip'), servers[other]);

    expect(await timed('disconnect', model.disconnect), isTrue);
    expect(model.connection.state, VpnState.idle);
    expect(await model.connect(), isTrue, reason: '${model.failure}');
    expect(await model.unsubscribe(), isTrue);
    expect(model.connection.state, VpnState.idle);
    expect(model.subscription, isNull);
    expect(model.snapshot.servers, isEmpty);

    await transport.close();
    expect(await transport.exitCode, 0);
    stdout.writeln('TIMES ${jsonEncode(times)}');
  }, timeout: const Timeout(Duration(minutes: 3)));
}
