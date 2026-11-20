import 'dart:async';
import 'dart:convert';
import 'dart:ffi';
import 'dart:isolate';

import 'client.dart';

class PipeRefused implements Exception {
  const PipeRefused();

  @override
  String toString() => 'the pipe belongs to someone other than the system';
}

class PipeTransport implements Transport {
  PipeTransport._(this._handle) : _stop = _createEvent(nullptr, 1, 0, nullptr) {
    _reads.listen(_read);
    _writes.listen(_written);
  }

  static Future<PipeTransport> connect(
    String path, {
    Duration limit = const Duration(seconds: 10),
    Duration step = const Duration(milliseconds: 50),
    bool Function(int handle) trust = systemOwned,
  }) async {
    final watch = Stopwatch()..start();
    while (true) {
      final handle = _open(path);
      if (handle != _invalid) {
        if (!trust(handle)) {
          _closeHandle(handle);
          throw const PipeRefused();
        }
        final transport = PipeTransport._(handle);
        await transport._start();
        return transport;
      }
      if (watch.elapsed >= limit) {
        throw StateError('the pipe $path is not there');
      }
      await Future<void>.delayed(step);
    }
  }

  final int _handle;
  final int _stop;
  final _reads = ReceivePort();
  final _writes = ReceivePort();
  final _bytes = StreamController<List<int>>();
  final _writer = Completer<SendPort>();
  final _readerDone = Completer<void>();
  final _writerDone = Completer<void>();
  Future<void>? _closing;

  @override
  late final Stream<String> lines = _bytes.stream.transform(utf8.decoder).transform(const LineSplitter());

  Future<void> _start() async {
    await Isolate.spawn(_reader, [_reads.sendPort, _handle, _stop]);
    await Isolate.spawn(_writerLoop, [_writes.sendPort, _handle, _stop]);
  }

  void _read(Object? message) {
    if (message is TransferableTypedData) {
      _bytes.add(message.materialize().asUint8List());
      return;
    }
    _readerDone.complete();
    _reads.close();
    unawaited(_bytes.close());
    unawaited(close());
  }

  void _written(Object? message) {
    if (message is SendPort) {
      _writer.complete(message);
    } else if (message == false) {
      unawaited(close());
    } else if (message == null) {
      _writerDone.complete();
      _writes.close();
    }
  }

  @override
  void send(String line) {
    if (_closing != null) {
      return;
    }
    final bytes = utf8.encode('$line\n');
    unawaited(_writer.future.then((port) => port.send(TransferableTypedData.fromList([bytes]))));
  }

  @override
  Future<void> close() => _closing ??= _shut();

  Future<void> _shut() async {
    _setEvent(_stop);
    (await _writer.future).send(null);
    await Future.wait([_readerDone.future, _writerDone.future]);
    _closeHandle(_handle);
    _closeHandle(_stop);
  }
}

const _invalid = -1;
const _pending = 0x103;
const _chunk = 64 << 10;

final _kernel = DynamicLibrary.open('kernel32.dll');
final _ntdll = DynamicLibrary.open('ntdll.dll');
final _advapi = DynamicLibrary.open('advapi32.dll');

final _createFile = _kernel
    .lookupFunction<
      IntPtr Function(Pointer<Uint16>, Uint32, Uint32, Pointer<Void>, Uint32, Uint32, IntPtr),
      int Function(Pointer<Uint16>, int, int, Pointer<Void>, int, int, int)
    >('CreateFileW', isLeaf: true);
typedef _NtIo = Int32 Function(
  IntPtr,
  IntPtr,
  Pointer<Void>,
  Pointer<Void>,
  Pointer<Uint8>,
  Pointer<Uint8>,
  Uint32,
  Pointer<Uint8>,
  Pointer<Void>,
);
typedef _DartIo = int Function(
  int,
  int,
  Pointer<Void>,
  Pointer<Void>,
  Pointer<Uint8>,
  Pointer<Uint8>,
  int,
  Pointer<Uint8>,
  Pointer<Void>,
);

final _ntRead = _ntdll.lookupFunction<_NtIo, _DartIo>('NtReadFile');
final _ntWrite = _ntdll.lookupFunction<_NtIo, _DartIo>('NtWriteFile');
final _resetEvent = _kernel.lookupFunction<Int32 Function(IntPtr), int Function(int)>('ResetEvent');
final _overlappedResult = _kernel
    .lookupFunction<
      Int32 Function(IntPtr, Pointer<Uint8>, Pointer<Uint32>, Int32),
      int Function(int, Pointer<Uint8>, Pointer<Uint32>, int)
    >('GetOverlappedResult');
final _createEvent = _kernel
    .lookupFunction<
      IntPtr Function(Pointer<Void>, Int32, Int32, Pointer<Void>),
      int Function(Pointer<Void>, int, int, Pointer<Void>)
    >('CreateEventW');
final _closeHandle = _kernel.lookupFunction<Int32 Function(IntPtr), int Function(int)>('CloseHandle');
final _cancelIo = _kernel.lookupFunction<Int32 Function(IntPtr, Pointer<Uint8>), int Function(int, Pointer<Uint8>)>(
  'CancelIoEx',
);
final _setEvent = _kernel.lookupFunction<Int32 Function(IntPtr), int Function(int)>('SetEvent');
final _waitAny = _kernel
    .lookupFunction<
      Uint32 Function(Uint32, Pointer<IntPtr>, Int32, Uint32),
      int Function(int, Pointer<IntPtr>, int, int)
    >('WaitForMultipleObjects');
final _processHeap = _kernel.lookupFunction<IntPtr Function(), int Function()>('GetProcessHeap');
final _heapAlloc = _kernel
    .lookupFunction<Pointer<Uint8> Function(IntPtr, Uint32, IntPtr), Pointer<Uint8> Function(int, int, int)>(
      'HeapAlloc',
    );
final _heapFree = _kernel
    .lookupFunction<Int32 Function(IntPtr, Uint32, Pointer<Uint8>), int Function(int, int, Pointer<Uint8>)>('HeapFree');
final _localFree = _kernel.lookupFunction<IntPtr Function(IntPtr), int Function(int)>('LocalFree');
final _securityInfo = _advapi
    .lookupFunction<
      Uint32 Function(
        IntPtr,
        Int32,
        Uint32,
        Pointer<IntPtr>,
        Pointer<IntPtr>,
        Pointer<IntPtr>,
        Pointer<IntPtr>,
        Pointer<IntPtr>,
      ),
      int Function(int, int, int, Pointer<IntPtr>, Pointer<IntPtr>, Pointer<IntPtr>, Pointer<IntPtr>, Pointer<IntPtr>)
    >('GetSecurityInfo');
final _wellKnown = _advapi.lookupFunction<Int32 Function(IntPtr, Int32), int Function(int, int)>('IsWellKnownSid');

Pointer<Uint8> _alloc(int size) {
  final memory = _heapAlloc(_processHeap(), 8, size);
  if (memory == nullptr) {
    throw StateError('out of memory');
  }
  return memory;
}

void _free(Pointer<Uint8> memory) => _heapFree(_processHeap(), 0, memory);

int _open(String path) {
  final units = path.codeUnits;
  final name = _alloc((units.length + 1) * 2);
  try {
    name.cast<Uint16>().asTypedList(units.length + 1).setAll(0, units);
    return _createFile(name.cast(), 0x0012008B, 0, nullptr, 3, 0x40110000, 0);
  } finally {
    _free(name);
  }
}

bool systemOwned(int handle) {
  final slots = _alloc(16).cast<IntPtr>();
  try {
    if (_securityInfo(handle, 6, 1, slots, nullptr, nullptr, nullptr, slots + 1) != 0) {
      return false;
    }
    final owner = slots.value;
    final trusted = _wellKnown(owner, 22) != 0 || _wellKnown(owner, 26) != 0;
    _localFree((slots + 1).value);
    return trusted;
  } finally {
    _free(slots.cast());
  }
}

final class _Io {
  _Io(this.handle, int stop, int size)
    : buffer = _alloc(size),
      overlapped = _alloc(32),
      count = _alloc(4).cast(),
      events = _alloc(16).cast() {
    events[0] = _createEvent(nullptr, 1, 0, nullptr);
    events[1] = stop;
    overlapped.cast<IntPtr>()[3] = events[0];
  }

  final int handle;
  Pointer<Uint8> buffer;
  final Pointer<Uint8> overlapped;
  final Pointer<Uint32> count;
  final Pointer<IntPtr> events;

  bool get stopped => _waitAny(1, events + 1, 0, 0) == 0;

  int run(_DartIo call, Pointer<Uint8> at, int length) {
    _resetEvent(events[0]);
    final status = call(handle, events[0], nullptr, nullptr, overlapped, at, length, overlapped + 16, nullptr);
    if (status < 0) {
      return -1;
    }
    if (status == _pending && _waitAny(2, events, 0, 0xFFFFFFFF) != 0) {
      _cancelIo(handle, overlapped);
      _overlappedResult(handle, overlapped, count, 1);
      return -1;
    }
    return overlapped.cast<Int32>()[0] < 0 ? -1 : overlapped.cast<IntPtr>()[1];
  }

  void dispose() {
    _closeHandle(events[0]);
    _free(buffer);
    _free(overlapped);
    _free(count.cast());
    _free(events.cast());
  }
}

void _reader(List<Object> start) {
  final out = start[0] as SendPort;
  final io = _Io(start[1] as int, start[2] as int, _chunk);
  try {
    while (!io.stopped) {
      final got = io.run(_ntRead, io.buffer, _chunk);
      if (got < 0) {
        break;
      }
      if (got > 0) {
        out.send(TransferableTypedData.fromList([io.buffer.asTypedList(got)]));
      }
    }
  } finally {
    io.dispose();
    out.send(null);
  }
}

void _writerLoop(List<Object> start) {
  final out = start[0] as SendPort;
  final inbox = ReceivePort();
  var size = 4096;
  final io = _Io(start[1] as int, start[2] as int, size);
  var broken = false;
  inbox.listen((message) {
    if (message is! TransferableTypedData) {
      io.dispose();
      inbox.close();
      out.send(null);
      return;
    }
    if (broken || io.stopped) {
      return;
    }
    final bytes = message.materialize().asUint8List();
    if (bytes.length > size) {
      _free(io.buffer);
      size = bytes.length;
      io.buffer = _alloc(size);
    }
    io.buffer.asTypedList(bytes.length).setAll(0, bytes);
    var done = 0;
    while (done < bytes.length) {
      final put = io.run(_ntWrite, io.buffer + done, bytes.length - done);
      if (put <= 0) {
        broken = true;
        out.send(false);
        return;
      }
      done += put;
    }
  });
  out.send(inbox.sendPort);
}
