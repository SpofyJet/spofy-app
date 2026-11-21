import 'dart:async';
import 'dart:ffi';
import 'dart:ui' show Color;

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
  bool get proxy => true;

  @override
  void Function(String address)? get browse => browseOnWindows;

  @override
  Future<bool> Function(String file)? get install => null;

  @override
  void Function(bool dark, Color background)? get dress => dressOnWindows;

  @override
  void Function()? get drawn => null;

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

void dressOnWindows(bool dark, Color background) {
  final heap = _processHeap();
  final memory = _heapAlloc(heap, 8, 64);
  if (memory == nullptr) {
    return;
  }
  try {
    final name = _windowClass.codeUnits;
    memory.asTypedList(name.length).setAll(0, name);
    final window = _findWindow(memory, nullptr);
    final value = (memory + 24).cast<Uint32>();
    if (window == 0 || _windowProcess(window, value) == 0 || value.value != _currentProcess()) {
      return;
    }
    final argb = background.toARGB32();
    final color = (argb & 0xFF) << 16 | (argb & 0xFF00) | (argb >> 16 & 0xFF);
    value.value = dark ? 1 : 0;
    _dwmSet(window, 20, value, 4);
    value.value = color;
    _dwmSet(window, 35, value, 4);
    _dwmSet(window, 34, value, 4);
    final brush = _brushes.putIfAbsent(color, () => _solidBrush(color));
    if (brush != 0) {
      _setClassLong(window, -10, brush);
    }
  } finally {
    _heapFree(heap, 0, memory);
  }
}

const _windowClass = 'SPOFY_WINDOW\u0000';
final _brushes = <int, int>{};

final _kernel = DynamicLibrary.open('kernel32.dll');
final _user = DynamicLibrary.open('user32.dll');
final _currentProcess = _kernel.lookupFunction<Uint32 Function(), int Function()>('GetCurrentProcessId');
final _findWindow = _user
    .lookupFunction<IntPtr Function(Pointer<Uint16>, Pointer<Uint16>), int Function(Pointer<Uint16>, Pointer<Uint16>)>(
      'FindWindowW',
    );
final _windowProcess = _user
    .lookupFunction<Uint32 Function(IntPtr, Pointer<Uint32>), int Function(int, Pointer<Uint32>)>(
      'GetWindowThreadProcessId',
    );
final _setClassLong = _user.lookupFunction<IntPtr Function(IntPtr, Int32, IntPtr), int Function(int, int, int)>(
  'SetClassLongPtrW',
);
final _solidBrush = DynamicLibrary.open('gdi32.dll')
    .lookupFunction<IntPtr Function(Uint32), int Function(int)>('CreateSolidBrush');
final _dwmSet = DynamicLibrary.open('dwmapi.dll')
    .lookupFunction<
      Int32 Function(IntPtr, Uint32, Pointer<Uint32>, Uint32),
      int Function(int, int, Pointer<Uint32>, int)
    >('DwmSetWindowAttribute');
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
