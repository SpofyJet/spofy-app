import 'dart:async';
import 'dart:ffi';

import '../core/client.dart';
import '../core/pipe_transport.dart';
import '../state/apps.dart';
import 'host.dart';

class WindowsHost implements Host {
  const WindowsHost();

  static const pipe = r'\\.\pipe\spofy';

  @override
  Future<Transport> open() => PipeTransport.connect(pipe, limit: const Duration(seconds: 20));

  @override
  Future<bool> permit() async => true;

  @override
  void Function()? get always => null;

  @override
  AppList? get apps => null;

  @override
  void Function(String address)? get browse => browseOnWindows;

  @override
  void links(void Function(String link) hear) {}
}

void browseOnWindows(String address) {
  final target = Uri.tryParse(address);
  if (target == null || target.scheme != 'https' || target.host.isEmpty) {
    return;
  }
  final text = target.toString().codeUnits;
  final verb = 'open'.codeUnits;
  final heap = _processHeap();
  final memory = _heapAlloc(heap, 8, (text.length + verb.length + 2) * 2);
  if (memory == nullptr) {
    return;
  }
  try {
    final units = memory.asTypedList(text.length + verb.length + 2);
    units.setAll(0, verb);
    units.setAll(verb.length + 1, text);
    _shellExecute(0, memory, memory + verb.length + 1, nullptr, nullptr, 1);
  } finally {
    _heapFree(heap, 0, memory);
  }
}

final _kernel = DynamicLibrary.open('kernel32.dll');
final _processHeap = _kernel.lookupFunction<IntPtr Function(), int Function()>('GetProcessHeap');
final _heapAlloc = _kernel
    .lookupFunction<Pointer<Uint16> Function(IntPtr, Uint32, IntPtr), Pointer<Uint16> Function(int, int, int)>(
      'HeapAlloc',
    );
final _heapFree = _kernel
    .lookupFunction<Int32 Function(IntPtr, Uint32, Pointer<Uint16>), int Function(int, int, Pointer<Uint16>)>(
      'HeapFree',
    );
final _shellExecute = DynamicLibrary.open('shell32.dll')
    .lookupFunction<
      IntPtr Function(IntPtr, Pointer<Uint16>, Pointer<Uint16>, Pointer<Uint16>, Pointer<Uint16>, Int32),
      int Function(int, Pointer<Uint16>, Pointer<Uint16>, Pointer<Uint16>, Pointer<Uint16>, int)
    >('ShellExecuteW');
