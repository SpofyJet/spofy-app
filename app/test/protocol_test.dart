import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/protocol.dart';

import 'transcript.dart';

Request? build(Map<String, Object?> sent) {
  final server = sent['server'] as String?;
  return switch (sent['op']) {
    'hello' => const Request.hello(),
    'subscribe' => Request.subscribe(sent['url']! as String),
    'refresh' => const Request.refresh(),
    'unsubscribe' => Request.unsubscribe(sent['subscription'] as String?),
    'activate' => Request.activate(sent['subscription']! as String),
    'logs' => const Request.logs(),
    'clearLogs' => const Request.clearLogs(),
    'checkUpdate' => const Request.checkUpdate(),
    'fetchUpdate' => const Request.fetchUpdate(),
    'installUpdate' => const Request.installUpdate(),
    'connect' => Request.connect(server),
    'disconnect' => const Request.disconnect(),
    'check' => const Request.check(),
    'online' => Request.online(online: sent['online']! as bool),
    'measure' => const Request.measure(),
    'set' => Request.set(sent['key']! as String, sent['value']),
    'traffic' => const Request.traffic(),
    'report' => const Request.report(),
    _ => null,
  };
}

void main() {
  final transcript = Transcript.load();

  test('the samples are big enough to mean something', () {
    expect(transcript.exchanges.length, greaterThanOrEqualTo(20));
    expect(transcript.exchanges.expand((e) => e.received).length, greaterThan(40));
  });

  test('every message of the core decodes strictly and encodes back to the same JSON', () {
    for (final exchange in transcript.exchanges) {
      for (final line in exchange.received) {
        final message = Message.parse(line);
        expect(jsonDecode(jsonEncode(message.toJson())), jsonDecode(line), reason: line);
      }
    }
  });

  test('every request of the samples is what the interface sends', () {
    var unknown = 0;
    for (final exchange in transcript.exchanges) {
      final sent = jsonDecode(exchange.sent) as Map<String, Object?>;
      final request = build(sent);
      if (request == null) {
        unknown++;
        expect((Message.parse(exchange.received.last) as Reply).code, 'unknown_op', reason: exchange.sent);
        continue;
      }
      expect(jsonDecode(jsonEncode(request.toJson(sent['id']! as int))), sent, reason: exchange.sent);
    }
    expect(unknown, 1);
  });

  test('every state and every event kind of the core appears in the samples or is known to the interface', () {
    final seen = <String>{};
    final kinds = <Type>{};
    for (final exchange in transcript.exchanges) {
      for (final line in exchange.received) {
        final message = Message.parse(line);
        kinds.add(message.runtimeType);
        if (message is ConnectionEvent) {
          seen.add(message.connection.state.name);
        }
      }
    }
    expect(kinds, containsAll([Reply, ConnectionEvent, SubscriptionEvent, ServersEvent, SettingsEvent]));
    expect(seen, containsAll(['idle', 'connecting', 'connected', 'switching', 'stalled', 'offline']));
    final go = File('../core/internal/session/session.go').readAsStringSync();
    final declared = RegExp(r'State = "([a-z]+)"').allMatches(go).map((m) => m.group(1)!).toSet();
    expect(declared, VpnState.values.map((s) => s.name).toSet());
  });

  test('a message with an unknown or missing field is refused', () {
    const good =
        '{"ev":"connection","connection":{"state":"idle","choice":"auto","server":"","delayMs":0,"code":"","error":"","since":0}}';
    expect(Message.parse(good), isA<ConnectionEvent>());
    for (final bad in [
      'not json',
      '[1]',
      '{}',
      '{"ev":"weather","weather":1}',
      '{"ev":"connection"}',
      good.replaceFirst('"error":""', '"error":"","extra":1'),
      good.replaceFirst('"delayMs":0,', ''),
      good.replaceFirst('"delayMs":0', '"delayMs":"0"'),
      good.replaceFirst('"idle"', '"asleep"'),
      '{"re":1}',
      '{"re":"1","ok":true}',
      '{"re":1,"ok":true,"surprise":1}',
      '{"ev":"servers","servers":{}}',
      '{"ev":"settings","settings":[]}',
    ]) {
      expect(() => Message.parse(bad), throwsA(isA<ProtocolError>()), reason: bad);
    }
  });
}
