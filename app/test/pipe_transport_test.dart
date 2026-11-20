@TestOn('windows')
library;

import 'dart:async';
import 'dart:convert';
import 'dart:ffi';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/client.dart';
import 'package:spofy/core/pipe_transport.dart';
import 'package:spofy/core/protocol.dart';
import 'package:spofy/state/model.dart';

bool anyOwner(int handle) => true;

void main() {
  var serial = 0;
  String name() => '\\\\.\\pipe\\spofy-dart-$pid-${serial++}';

  Future<(Process, String)> core() async {
    final path = Platform.environment['SPOFY_CORE'];
    if (path == null) {
      throw StateError('SPOFY_CORE is not set');
    }
    final folder = Directory.systemTemp.createTempSync('spofy-pipe-');
    final pipe = name();
    final process = await Process.start(
      path,
      ['serve', '-state', '${folder.path}\\state', '-mode', 'proxy', '-listen', pipe],
      environment: {'SPOFY_STATE_KEY': base64Encode(List<int>.generate(32, (i) => i * 3 % 251))},
    );
    addTearDown(() async {
      process.kill();
      await process.exitCode;
      folder.deleteSync(recursive: true);
    });
    unawaited(process.stderr.drain<void>());
    final listening = await process.stdout.transform(utf8.decoder).transform(const LineSplitter()).first;
    expect(listening, contains('listening'));
    return (process, pipe);
  }

  test('the core on a pipe serves two windows at once', () async {
    final (process, pipe) = await core();
    final one = AppModel(CoreClient(await PipeTransport.connect(pipe, trust: anyOwner)));
    final two = AppModel(CoreClient(await PipeTransport.connect(pipe, trust: anyOwner)));
    await one.start();
    await two.start();
    expect(one.ready && two.ready, isTrue);
    await one.set('theme', 'dark');
    await two.check();
    expect(two.snapshot.settings, {'theme': 'dark'});
    await process.stdin.close();
    expect(await process.exitCode.timeout(const Duration(seconds: 10)), 0);
    await pumpEventQueue();
    await two.check();
    expect(one.fault, AppModel.coreClosed);
    expect(two.fault, AppModel.coreClosed);
  });

  test('a long request and many quick ones arrive whole and in order', () async {
    final (_, pipe) = await core();
    final client = CoreClient(await PipeTransport.connect(pipe, trust: anyOwner));
    final names = [for (var i = 0; i < 500; i++) 'app.example.n${i.toString().padLeft(4, '0')}'];
    final replies = await Future.wait([
      for (var i = 0; i < 200; i++) client.call(Request.set('n$i', i)),
      client.call(Request.set('apps', {'mode': 'except', 'list': names})),
    ]);
    expect(replies.every((reply) => reply.ok), isTrue);
    final hello = await client.call(const Request.hello());
    expect(hello.snapshot?.settings['n199'], 199);
    expect(hello.snapshot?.settings['apps'], {'mode': 'except', 'list': names});
    await client.close();
  });

  test('closing the window side is quick and ends its lines', () async {
    final (_, pipe) = await core();
    final transport = await PipeTransport.connect(pipe, trust: anyOwner);
    final ended = Completer<void>();
    transport.lines.listen((_) {}, onDone: ended.complete);
    final watch = Stopwatch()..start();
    await transport.close().timeout(const Duration(seconds: 2));
    await ended.future.timeout(const Duration(seconds: 2));
    expect(watch.elapsed, lessThan(const Duration(seconds: 2)));
    transport.send('{"id":1,"op":"hello"}');
  });

  test('the core killed ends the lines at once', () async {
    final (process, pipe) = await core();
    final transport = await PipeTransport.connect(pipe, trust: anyOwner);
    final ended = Completer<void>();
    transport.lines.listen((_) {}, onDone: ended.complete);
    process.kill();
    await ended.future.timeout(const Duration(seconds: 5));
    await transport.close().timeout(const Duration(seconds: 2));
  });

  test('no pipe: the window waits for it up to the limit', () async {
    final watch = Stopwatch()..start();
    await expectLater(PipeTransport.connect(name(), limit: const Duration(milliseconds: 300)), throwsStateError);
    expect(watch.elapsed, greaterThanOrEqualTo(const Duration(milliseconds: 300)));
  });

  test('a pipe set up by an ordinary account is refused before anything is sent', () async {
    final who = await Process.run('whoami', ['/user', '/fo', 'csv', '/nh']);
    final sid = RegExp(r'"(S-1-5-[0-9-]+)"').firstMatch('${who.stdout}')!.group(1)!;
    final pipe = name();
    final server = _squat(pipe, 'O:${sid}D:P(A;;GA;;;WD)');
    addTearDown(() => _closeHandle(server));
    await expectLater(PipeTransport.connect(pipe), throwsA(isA<PipeRefused>()));
  });
}

final _kernel = DynamicLibrary.open('kernel32.dll');
final _closeHandle = _kernel.lookupFunction<Int32 Function(IntPtr), int Function(int)>('CloseHandle');

int _squat(String pipe, String access) {
  final heap = _kernel.lookupFunction<IntPtr Function(), int Function()>('GetProcessHeap')();
  final alloc = _kernel
      .lookupFunction<Pointer<Uint8> Function(IntPtr, Uint32, IntPtr), Pointer<Uint8> Function(int, int, int)>(
        'HeapAlloc',
      );
  final parse = DynamicLibrary.open('advapi32.dll')
      .lookupFunction<
        Int32 Function(Pointer<Uint16>, Uint32, Pointer<IntPtr>, Pointer<Uint32>),
        int Function(Pointer<Uint16>, int, Pointer<IntPtr>, Pointer<Uint32>)
      >('ConvertStringSecurityDescriptorToSecurityDescriptorW');
  final create = _kernel
      .lookupFunction<
        IntPtr Function(Pointer<Uint16>, Uint32, Uint32, Uint32, Uint32, Uint32, Uint32, Pointer<Uint8>),
        int Function(Pointer<Uint16>, int, int, int, int, int, int, Pointer<Uint8>)
      >('CreateNamedPipeW');
  Pointer<Uint16> text(String value) {
    final memory = alloc(heap, 8, (value.length + 1) * 2).cast<Uint16>();
    memory.asTypedList(value.length).setAll(0, value.codeUnits);
    return memory;
  }

  final descriptor = alloc(heap, 8, 8).cast<IntPtr>();
  expect(parse(text(access), 1, descriptor, nullptr), isNot(0));
  final attributes = alloc(heap, 8, 24);
  attributes.cast<Uint32>().value = 24;
  (attributes + 8).cast<IntPtr>().value = descriptor.value;
  final handle = create(text(pipe), 3 | 0x40000000, 0, 1, 4096, 4096, 0, attributes);
  expect(handle, isNot(-1));
  return handle;
}
