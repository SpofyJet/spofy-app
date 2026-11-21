import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:flutter/services.dart';

import '../core/client.dart';
import '../core/process_transport.dart';
import '../core/socket_transport.dart';
import '../state/apps.dart';
import 'windows.dart';

abstract interface class Host {
  Future<Transport> open();

  Future<bool> permit();

  void Function()? get always;

  AppList? get apps;

  void Function(String address)? get browse;

  Future<bool> Function(String file)? get install;

  void Function(bool dark, Color background)? get dress;

  void Function()? get drawn;

  void links(void Function(String link) hear);
}

Host host() =>
    Platform.isAndroid ? const AndroidHost() : (Platform.isWindows ? const WindowsHost() : const DesktopHost());

class AndroidHost implements Host {
  const AndroidHost();

  static const _channel = MethodChannel('app.spofy/host');

  @override
  Future<Transport> open() async {
    final path = await _channel.invokeMethod<String>('start');
    return SocketTransport.connect(path!);
  }

  @override
  Future<bool> permit() async {
    return await _channel.invokeMethod<bool>('prepare') ?? false;
  }

  @override
  void Function()? get always =>
      () => unawaited(_channel.invokeMethod<void>('always'));

  @override
  AppList? get apps => const _AndroidApps();

  @override
  void Function(String address)? get browse =>
      (address) => unawaited(_channel.invokeMethod<void>('browse', address));

  @override
  Future<bool> Function(String file)? get install => (file) async {
    try {
      return await _channel.invokeMethod<bool>('install', file) ?? false;
    } on PlatformException {
      return false;
    }
  };

  @override
  void Function(bool dark, Color background)? get dress => null;

  @override
  void Function()? get drawn =>
      () => unawaited(_channel.invokeMethod<void>('drawn'));

  @override
  void links(void Function(String link) hear) {
    _channel.setMethodCallHandler((call) async {
      final link = call.arguments;
      if (call.method == 'link' && link is String) {
        hear(link);
      }
    });
    unawaited(_first(hear));
  }

  Future<void> _first(void Function(String link) hear) async {
    try {
      final link = await _channel.invokeMethod<String>('link');
      if (link != null) {
        hear(link);
      }
    } on PlatformException {
      return;
    } on MissingPluginException {
      return;
    }
  }
}

class _AndroidApps implements AppList {
  const _AndroidApps();

  @override
  Future<List<InstalledApp>> installed() async {
    final found = await AndroidHost._channel.invokeListMethod<Map<Object?, Object?>>('apps') ?? const [];
    return [
      for (final app in found)
        if (app['id'] case final String id)
          InstalledApp(id, switch (app['name']) {
            final String name when name.isNotEmpty => name,
            _ => id,
          }),
    ];
  }

  @override
  Future<Uint8List?> icon(String id) => AndroidHost._channel.invokeMethod<Uint8List>('icon', id);

  @override
  Future<bool> locked() async => await AndroidHost._channel.invokeMethod<bool>('locked') ?? false;
}

class DesktopHost implements Host {
  const DesktopHost();

  static String _home() => Platform.environment['HOME'] ?? Directory.systemTemp.path;

  static String _key() {
    final given = Platform.environment['SPOFY_STATE_KEY'];
    if (given != null && given.isNotEmpty) {
      return given;
    }
    final file = File('${Platform.environment['XDG_CONFIG_HOME'] ?? '${_home()}/.config'}/spofy/key');
    if (file.existsSync()) {
      return file.readAsStringSync().trim();
    }
    final random = Random.secure();
    final key = base64Encode(List<int>.generate(32, (_) => random.nextInt(256)));
    file.parent.createSync(recursive: true);
    file.writeAsStringSync(key, flush: true);
    Process.runSync('chmod', ['600', file.path]);
    return key;
  }

  @override
  Future<Transport> open() {
    final core = Platform.environment['SPOFY_CORE'] ?? '${File(Platform.resolvedExecutable).parent.path}/spofy-core';
    final state =
        Platform.environment['SPOFY_STATE'] ??
        '${Platform.environment['XDG_DATA_HOME'] ?? '${_home()}/.local/share'}/spofy';
    final mode = Platform.environment['SPOFY_MODE'] ?? 'tun';
    return ProcessTransport.start(
      core,
      ['serve', '-state', state, '-mode', mode],
      environment: {'SPOFY_STATE_KEY': _key()},
    );
  }

  @override
  Future<bool> permit() async => true;

  @override
  void Function()? get always => null;

  @override
  AppList? get apps => null;

  @override
  void Function(String address)? get browse =>
      (address) => unawaited(_browse(address));

  static Future<void> _browse(String address) async {
    try {
      await Process.start(Platform.isMacOS ? 'open' : 'xdg-open', [address], mode: ProcessStartMode.detached);
    } on ProcessException {
      return;
    }
  }

  @override
  Future<bool> Function(String file)? get install => null;

  @override
  void Function(bool dark, Color background)? get dress => null;

  @override
  void Function()? get drawn => null;

  @override
  void links(void Function(String link) hear) {}
}
