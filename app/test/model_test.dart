import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/client.dart';
import 'package:spofy/core/process_transport.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/state/model.dart';

import 'replay.dart';
import 'transcript.dart';

Object? normal(Object? json) => jsonDecode(jsonEncode(json));

void main() {
  test('the model follows the whole sample session and ends with the state the core reports', () async {
    final transcript = Transcript.load();
    final transport = ReplayTransport(transcript.exchanges);
    final model = AppModel(CoreClient(transport));
    var changes = 0;
    model.addListener(() => changes++);
    var compared = 0;
    var failures = 0;
    while (transport.position < transcript.exchanges.length) {
      final exchange = transport.next;
      final sent = jsonDecode(exchange.sent) as Map<String, Object?>;
      final reply = Message.parse(exchange.received.last) as Reply;
      final server = sent['server'] as String?;
      switch (sent['op']) {
        case 'hello':
          if (model.ready) {
            expect(
              normal(model.snapshot.toJson()),
              normal(reply.snapshot!.toJson()),
              reason: 'before request ${sent['id']}',
            );
            compared++;
          }
          await model.start();
        case 'subscribe':
          expect(await model.subscribe(sent['url']! as String), reply.ok);
        case 'refresh':
          expect(await model.refresh(), reply.ok);
        case 'unsubscribe':
          expect(await model.unsubscribe(), reply.ok);
        case 'connect':
          expect(await model.connect(server), reply.ok);
        case 'disconnect':
          expect(await model.disconnect(), reply.ok);
        case 'measure':
          expect(await model.measure(), reply.ok);
        case 'check':
          await model.check();
        case 'online':
          await model.online(online: sent['online']! as bool);
        case 'set':
          await model.set(sent['key']! as String, sent['value']);
        case 'traffic':
          await model.poll();
          expect(normal(model.traffic.toJson()), normal(reply.traffic!.toJson()));
        default:
          transport.skip();
          continue;
      }
      if (!reply.ok && const {'subscribe', 'refresh', 'connect', 'measure'}.contains(sent['op'])) {
        expect(model.failure?.code, reply.code, reason: exchange.sent);
        failures++;
      }
    }
    expect(compared, 2);
    expect(failures, greaterThanOrEqualTo(4));
    expect(changes, greaterThan(40));
    expect(model.fault, '');
    expect(model.subscription, isNull);
    expect(model.connection.state, VpnState.idle);
  });

  test('events reach the listener before the reply of the same request', () async {
    final transcript = Transcript.load();
    final transport = ReplayTransport(transcript.exchanges);
    final client = CoreClient(transport);
    final order = <String>[];
    client.events.listen((e) => order.add(e.runtimeType.toString()));
    await client.call(const Request.hello());
    transport.skip();
    transport.skip();
    final reply = await client.call(Request.subscribe('https://sub.example.test/sub/TESTTOKEN0123456789'));
    order.add('reply');
    expect(reply.ok, isTrue);
    expect(order, ['SubscriptionEvent', 'ServersEvent', 'reply']);
  });

  test('a broken line from the core is reported and does not stop the session', () async {
    final transcript = Transcript.load();
    final transport = ReplayTransport(transcript.exchanges);
    final model = AppModel(CoreClient(transport));
    await model.start();
    transport.inject('{"ev":"connection","connection":{"state":"flying"}}');
    await pumpEventQueue();
    expect(model.fault, AppModel.coreProtocol);
    expect(model.connection.state, VpnState.idle);
  });

  test('when the core goes away pending requests end and the model says so', () async {
    final transport = ReplayTransport([Exchange('{"id":1,"op":"hello"}')]);
    final client = CoreClient(transport);
    final model = AppModel(client);
    final pending = model.start();
    await transport.close();
    await pending;
    expect(model.ready, isFalse);
    expect(model.fault, AppModel.coreClosed);
    expect(client.closed, isTrue);
    await expectLater(client.call(const Request.hello()), throwsA(isA<CoreClosed>()));
    expect(await model.connect(), isFalse);
  });

  test('a second tap while the first request is running sends nothing', () async {
    final transcript = Transcript.load();
    final transport = ReplayTransport(transcript.exchanges);
    final model = AppModel(CoreClient(transport));
    await model.start();
    final first = model.connect();
    expect(model.running(Task.connect), isTrue);
    expect(await model.connect(), isFalse);
    expect(await first, isFalse);
    expect(model.running(Task.connect), isFalse);
    expect(transport.position, 2);
  });

  test('the real core answers over its channel', () async {
    final path = Platform.environment['SPOFY_CORE'];
    if (path == null) {
      markTestSkipped('SPOFY_CORE is not set');
      return;
    }
    final folder = Directory.systemTemp.createTempSync('spofy-model-');
    addTearDown(() => folder.deleteSync(recursive: true));
    final key = base64Encode(List<int>.generate(32, (i) => i * 7 % 251));
    Future<ProcessTransport> launch() => ProcessTransport.start(
      path,
      ['serve', '-state', folder.path, '-mode', 'proxy'],
      environment: {'SPOFY_STATE_KEY': key},
    );

    var transport = await launch();
    var model = AppModel(CoreClient(transport));
    await model.start();
    expect(model.ready, isTrue);
    expect(model.fault, '');
    expect(model.snapshot.protocol, protocolVersion);
    expect(model.snapshot.version, isNotEmpty);
    expect(model.subscription, isNull);
    expect(await model.connect(), isFalse);
    expect(model.failure?.code, 'no_subscription');
    expect(await model.subscribe('http://sub.example.test/x'), isFalse);
    expect(model.failure?.code, 'sub_bad_url');
    await model.set('killSwitch', true);
    await model.set('apps', ['org.example.one']);
    expect(model.snapshot.settings, {
      'killSwitch': true,
      'apps': ['org.example.one'],
    });
    await transport.close();
    expect(await transport.exitCode, 0);

    transport = await launch();
    model = AppModel(CoreClient(transport));
    await model.start();
    expect(model.snapshot.settings, {
      'killSwitch': true,
      'apps': ['org.example.one'],
    });
    await transport.close();
    expect(await transport.exitCode, 0);
    expect(File('${folder.path}/state').readAsBytesSync().length, greaterThan(40));
    expect(
      utf8.decode(File('${folder.path}/state').readAsBytesSync(), allowMalformed: true).contains('killSwitch'),
      isFalse,
    );
  });
}
