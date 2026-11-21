import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/client.dart';
import 'package:spofy/core/process_transport.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/core/socket_transport.dart';
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
          expect(await model.unsubscribe(sent['subscription'] as String?), reply.ok);
        case 'checkUpdate':
          expect(await model.checkUpdate(), reply.ok);
        case 'fetchUpdate':
          expect(await model.fetchUpdate(), reply.ok);
          expect(model.update?.ready, isTrue);
        case 'installUpdate':
          expect(await model.installUpdate(), reply.ok);
        case 'activate':
          expect(await model.activate(sent['subscription']! as String), reply.ok);
        case 'logs':
          expect([for (final l in (await model.logs())!) l.toJson()], [for (final l in reply.logs!) l.toJson()]);
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
          expect(await model.set(sent['key']! as String, sent['value']), reply.ok);
        case 'traffic':
          await model.poll();
          expect(normal(model.traffic.toJson()), normal(reply.traffic!.toJson()));
        default:
          transport.skip();
          continue;
      }
      if (!reply.ok &&
          const {
            'subscribe',
            'refresh',
            'connect',
            'measure',
            'activate',
            'checkUpdate',
            'fetchUpdate',
            'installUpdate',
          }.contains(sent['op'])) {
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
    expect(order, ['SubscriptionEvent', 'ServersEvent', 'SubscriptionsEvent', 'reply']);
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

  test('a lost channel is opened again and the model takes a fresh snapshot', () async {
    final transcript = Transcript.load();
    final hello = transcript.exchanges.first;
    final first = ReplayTransport([hello]);
    final later = <ReplayTransport>[];
    var refuse = false;
    final model = AppModel(
      CoreClient(first),
      reopen: () async {
        if (refuse) {
          throw const CoreClosed();
        }
        later.add(ReplayTransport([hello, hello]));
        return CoreClient(later.last);
      },
    );
    await model.start();
    expect(model.ready, isTrue);
    await first.close();
    await pumpEventQueue();
    expect(later, hasLength(1));
    expect(later.single.position, 1);
    expect(model.fault, '');
    await model.revive();
    expect(later, hasLength(1));
    refuse = true;
    await later.single.close();
    await pumpEventQueue();
    expect(model.fault, AppModel.coreClosed);
    refuse = false;
    await model.revive();
    expect(model.fault, '');
    expect(later, hasLength(2));
    expect(later.last.position, 1);
  });

  test('the model opens the core itself: no snapshot until it answers, a fault when it does not come', () async {
    final hello = Transcript.load().exchanges.first;
    final gate = Completer<CoreClient>();
    final model = AppModel.opening(() => gate.future);
    expect(model.ready, isFalse);
    expect(await model.connect(), isFalse);
    expect(model.failure, isNull);
    final transport = ReplayTransport([hello]);
    gate.complete(CoreClient(transport));
    await pumpEventQueue();
    expect(model.ready, isTrue);
    expect(model.fault, '');
    expect(transport.position, 1);
    final absent = AppModel.opening(() async => throw const CoreClosed());
    await pumpEventQueue();
    expect(absent.ready, isFalse);
    expect(absent.fault, AppModel.coreClosed);
  });

  test('an app link offers a subscription address and never adds it by itself', () async {
    const address = 'https://sub.example.test/sub/TESTTOKEN0123456789';
    expect(offeredAddress('spofy://add/$address'), address);
    expect(offeredAddress(' spofy://add?url=${Uri.encodeQueryComponent(address)} '), address);
    expect(offeredAddress('spofy://add/$address?x=1#frag'), '$address?x=1#frag');
    for (final bad in [
      '',
      address,
      'spofy://add',
      'spofy://add/',
      'spofy://add/http://sub.example.test/sub/x',
      'spofy://add?url=file:///etc/passwd',
      'spofy://remove/$address',
      'spofy://addmore/$address',
      'other://add/$address',
      'spofy://add/https://',
      'spofy://add/https://sub.example.test/${'x' * 2100}',
    ]) {
      expect(offeredAddress(bad), isNull, reason: bad);
    }
    final transport = ReplayTransport(Transcript.load().exchanges);
    final model = AppModel(CoreClient(transport));
    await model.start();
    var changes = 0;
    model.addListener(() => changes++);
    model.offer('spofy://add/http://sub.example.test/x');
    expect(model.offered, isNull);
    expect(changes, 0);
    model.offer('spofy://add/$address');
    expect(model.offered, address);
    expect(changes, 1);
    expect(transport.position, 1);
    model.decline();
    expect(model.offered, isNull);
    expect(changes, 2);
    model.offer('spofy://add/$address');
    expect(model.take(), address);
    expect(model.take(), isNull);
    expect(transport.position, 1);
  });

  test('connect asks for the permission first, a refusal sends nothing and is kept as the reason', () async {
    final transcript = Transcript.load();
    final transport = ReplayTransport(transcript.exchanges);
    var allowed = false;
    var asked = 0;
    final model = AppModel(
      CoreClient(transport),
      permit: () async {
        asked++;
        return allowed;
      },
    );
    await model.start();
    expect(await model.connect(), isFalse);
    expect(model.failure?.code, AppModel.denied);
    expect(transport.position, 1);
    allowed = true;
    await model.connect();
    expect(asked, 2);
    expect(transport.position, 2);
    expect(model.failure?.code, isNot(AppModel.denied));
  });

  test('the real core serves two clients on its socket and stops when its input closes', () async {
    final path = Platform.environment['SPOFY_CORE'];
    if (path == null) {
      markTestSkipped('SPOFY_CORE is not set');
      return;
    }
    final folder = Directory.systemTemp.createTempSync('spofy-socket-');
    addTearDown(() => folder.deleteSync(recursive: true));
    final socket = '${folder.path}/core.sock';
    final core = await Process.start(
      path,
      ['serve', '-state', '${folder.path}/state', '-mode', 'none', '-listen', socket],
      environment: {'SPOFY_STATE_KEY': base64Encode(List<int>.generate(32, (i) => i * 5 % 251))},
    );
    unawaited(core.stderr.drain<void>());
    final listening = await core.stdout.transform(utf8.decoder).transform(const LineSplitter()).first;
    expect(listening, contains('listening'));
    final one = AppModel(CoreClient(await SocketTransport.connect(socket)));
    final two = AppModel(CoreClient(await SocketTransport.connect(socket)));
    await one.start();
    await two.start();
    expect(one.ready && two.ready, isTrue);
    await one.set('theme', 'dark');
    await two.check();
    expect(two.snapshot.settings, {'theme': 'dark'});
    await core.stdin.close();
    expect(await core.exitCode.timeout(const Duration(seconds: 10)), 0);
    await pumpEventQueue();
    await two.check();
    expect(one.fault, AppModel.coreClosed);
    expect(two.fault, AppModel.coreClosed);
    expect(File(socket).existsSync(), isFalse);
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
      ['serve', '-state', folder.path, '-mode', 'none'],
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
    await model.set('apps', {
      'mode': 'except',
      'list': ['org.example.one'],
    });
    expect(model.snapshot.settings, {
      'killSwitch': true,
      'apps': {
        'mode': 'except',
        'list': ['org.example.one'],
      },
    });
    await transport.close();
    expect(await transport.exitCode, 0);

    transport = await launch();
    model = AppModel(CoreClient(transport));
    await model.start();
    expect(model.snapshot.settings, {
      'killSwitch': true,
      'apps': {
        'mode': 'except',
        'list': ['org.example.one'],
      },
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
