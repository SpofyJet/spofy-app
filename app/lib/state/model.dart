import 'dart:async';

import 'package:flutter/foundation.dart';

import '../core/client.dart';
import '../core/protocol.dart';
import 'apps.dart';

enum Task {
  subscribe,
  refresh,
  unsubscribe,
  activate,
  connect,
  disconnect,
  measure,
  checkUpdate,
  fetchUpdate,
  installUpdate,
}

class Failure {
  const Failure(this.task, this.code, this.error);

  final Task task;
  final String code;
  final String error;
}

class AppModel extends ChangeNotifier {
  AppModel(
    CoreClient client, {
    this.reopen,
    this.permit,
    this.always,
    this.apps,
    this.proxy = false,
    this.browse,
    this.install,
    this.clock = DateTime.now,
  }) {
    _attach(client);
  }

  AppModel.opening(
    Future<CoreClient> Function() open, {
    this.permit,
    this.always,
    this.apps,
    this.proxy = false,
    this.browse,
    this.install,
    this.clock = DateTime.now,
  }) : reopen = open {
    unawaited(revive());
  }

  static const coreClosed = 'core_closed';
  static const coreProtocol = 'core_protocol';
  static const denied = 'vpn_denied';

  final Future<CoreClient> Function()? reopen;
  final Future<bool> Function()? permit;
  final VoidCallback? always;
  final AppList? apps;
  final bool proxy;
  final void Function(String address)? browse;
  final Future<bool> Function(String file)? install;
  final DateTime Function() clock;

  CoreClient? _client;
  StreamSubscription<Event>? _events;
  StreamSubscription<Object>? _faults;
  final _running = <Task>{};
  bool _disposed = false;
  bool _reviving = false;
  (Traffic, DateTime)? _sample;
  int _samples = 0;

  Snapshot snapshot = const Snapshot();
  Traffic traffic = const Traffic();
  Traffic? speed;
  bool ready = false;
  String fault = '';
  Failure? failure;
  String? offered;

  void offer(String link) {
    final target = offeredAddress(link);
    if (target == null || _disposed) {
      return;
    }
    offered = target;
    notifyListeners();
  }

  String? take() {
    final target = offered;
    offered = null;
    return target;
  }

  void decline() {
    if (take() != null && !_disposed) {
      notifyListeners();
    }
  }

  Connection get connection => snapshot.connection;

  Subscription? get subscription => snapshot.subscription;

  List<Brief> get subscriptions => snapshot.subscriptions;

  Update? get update => snapshot.update;

  Future<bool> checkUpdate() => _run(Task.checkUpdate, const Request.checkUpdate());

  Future<bool> fetchUpdate() => _run(Task.fetchUpdate, const Request.fetchUpdate());

  Future<bool> installUpdate() async {
    final ready = update;
    if (ready == null || !ready.ready) {
      return false;
    }
    final own = install;
    if (own != null) {
      final ok = await own(ready.file);
      if (!ok) {
        failure = const Failure(Task.installUpdate, 'update_install', '');
        _changed();
      }
      return ok;
    }
    return _run(Task.installUpdate, const Request.installUpdate());
  }

  AppsChoice get chosenApps => AppsChoice.of(snapshot.settings[appsKey]);

  bool running(Task task) => _running.contains(task);

  bool get reviving => _reviving;

  Server? get current => snapshot.server(connection.server);

  Future<void> start() async {
    final reply = await _ask(const Request.hello());
    final seen = reply?.snapshot;
    if (seen == null) {
      return;
    }
    if (seen.protocol != protocolVersion) {
      fault = coreProtocol;
    }
    snapshot = seen;
    if (seen.connection.state.protecting) {
      final series = _samples;
      final counted = (await _ask(const Request.traffic()))?.traffic;
      if (counted != null && series == _samples) {
        traffic = counted;
        _sample = (counted, clock());
      }
    }
    ready = true;
    _changed();
  }

  Future<bool> subscribe(String url) => _run(Task.subscribe, Request.subscribe(url.trim()));

  Future<bool> refresh() => _run(Task.refresh, const Request.refresh());

  Future<bool> unsubscribe([String? id]) => _run(Task.unsubscribe, Request.unsubscribe(id));

  Future<bool> activate(String id) => _run(Task.activate, Request.activate(id));

  Future<bool> measure() => _run(Task.measure, const Request.measure());

  Future<bool> connect([String? server]) async {
    final ask = permit;
    if (ask != null && !_running.contains(Task.connect) && !await ask()) {
      failure = const Failure(Task.connect, denied, '');
      _changed();
      return false;
    }
    return _run(Task.connect, Request.connect(server));
  }

  Future<void> revive() async {
    final open = reopen;
    if (open == null || _reviving || _disposed || !(_client?.closed ?? true)) {
      return;
    }
    _reviving = true;
    _changed();
    try {
      final client = await open();
      if (_disposed) {
        await client.close();
        return;
      }
      _attach(client);
      _running.clear();
      if (fault == coreClosed) {
        fault = '';
      }
      await start();
    } on Object {
      if (fault.isEmpty) {
        fault = coreClosed;
      }
    } finally {
      _reviving = false;
      _changed();
    }
  }

  Future<bool> disconnect() => _run(Task.disconnect, const Request.disconnect());

  Future<bool> toggle() {
    final state = connection.state;
    if (state == VpnState.idle || state == VpnState.failed) {
      return connect();
    }
    return disconnect();
  }

  Future<bool> choose(String server) {
    if (connection.state == VpnState.idle || connection.state == VpnState.failed) {
      snapshot = snapshot.apply(
        ConnectionEvent(
          Connection(
            state: connection.state,
            choice: server,
            server: connection.server,
            delayMs: connection.delayMs,
            code: connection.code,
            error: connection.error,
          ),
        ),
      );
      _changed();
    }
    return connect(server);
  }

  Future<void> check() async {
    await _ask(const Request.check());
  }

  Future<void> online({required bool online}) async {
    await _ask(Request.online(online: online));
  }

  static const reconnecting = {'mode', 'fragment', 'dns', 'ipv6', 'lan', 'logLevel'};

  (String, Object?)? pending;

  Future<bool> set(String key, Object? value) async {
    final state = connection.state;
    if (reconnecting.contains(key) && (state.protecting || state == VpnState.connecting)) {
      pending = (key, value);
      notifyListeners();
      return false;
    }
    return _set(key, value);
  }

  Future<bool> apply() async {
    final change = pending;
    if (change == null) {
      return false;
    }
    pending = null;
    notifyListeners();
    return _set(change.$1, change.$2);
  }

  void keep() {
    if (pending != null) {
      pending = null;
      notifyListeners();
    }
  }

  Future<bool> _set(String key, Object? value) async => (await _ask(Request.set(key, value)))?.ok ?? false;

  Future<Report?> report() async => (await _ask(const Request.report()))?.report;

  Future<List<LogLine>?> logs() async => (await _ask(const Request.logs()))?.logs;

  Future<void> clearLogs() async {
    await _ask(const Request.clearLogs());
  }

  bool get sampled => _sample != null;

  Future<void> poll() async {
    final series = _samples;
    final seen = (await _ask(const Request.traffic()))?.traffic;
    if (seen == null || series != _samples) {
      return;
    }
    final at = clock();
    final last = _sample;
    _sample = (seen, at);
    final ms = last == null ? 0 : at.difference(last.$2).inMilliseconds;
    final rate = last == null || ms <= 0 || seen.up < last.$1.up || seen.down < last.$1.down
        ? null
        : Traffic(up: (seen.up - last.$1.up) * 1000 ~/ ms, down: (seen.down - last.$1.down) * 1000 ~/ ms);
    if (seen.up != traffic.up || seen.down != traffic.down || rate?.up != speed?.up || rate?.down != speed?.down) {
      traffic = seen;
      speed = rate;
      _changed();
    }
  }

  void rest() {
    _forget();
    if (speed != null) {
      speed = null;
      _changed();
    }
  }

  void _forget() {
    _sample = null;
    _samples++;
  }

  void dismiss() {
    if (failure != null) {
      failure = null;
      _changed();
    }
  }

  Future<void> close() async {
    _disposed = true;
    await _events?.cancel();
    await _faults?.cancel();
    await _client?.close();
  }

  @override
  void dispose() {
    _disposed = true;
    _events?.cancel();
    _faults?.cancel();
    super.dispose();
  }

  void _attach(CoreClient client) {
    _events?.cancel();
    _faults?.cancel();
    _client = client;
    _events = client.events.listen(_apply, onDone: _lost);
    _faults = client.faults.listen(_broken);
  }

  Future<bool> _run(Task task, Request request) async {
    if (_running.contains(task)) {
      return false;
    }
    _running.add(task);
    failure = null;
    _changed();
    final reply = await _ask(request);
    _running.remove(task);
    final ok = reply?.ok ?? false;
    if (reply != null && !ok && reply.code != 'canceled') {
      failure = Failure(task, reply.code, reply.error);
    }
    _changed();
    return ok;
  }

  Future<Reply?> _ask(Request request) async {
    final client = _client;
    if (client == null) {
      return null;
    }
    try {
      return await client.call(request);
    } on CoreClosed {
      _lost();
      return null;
    }
  }

  void _apply(Event event) {
    snapshot = snapshot.apply(event);
    if (event is ConnectionEvent && !event.connection.state.protecting) {
      traffic = const Traffic();
      speed = null;
      _forget();
    }
    _changed();
  }

  void _broken(Object error) {
    fault = coreProtocol;
    _changed();
  }

  void _lost() {
    if (reopen != null) {
      unawaited(revive());
      return;
    }
    if (fault.isEmpty) {
      fault = coreClosed;
      _changed();
    }
  }

  void _changed() {
    if (!_disposed) {
      notifyListeners();
    }
  }
}

String? offeredAddress(String link) {
  const prefix = 'spofy://add';
  final text = link.trim();
  if (!text.startsWith(prefix)) {
    return null;
  }
  final rest = text.substring(prefix.length);
  var target = '';
  if (rest.startsWith('/')) {
    target = rest.substring(1);
  } else if (rest.startsWith('?')) {
    target = Uri.tryParse(text)?.queryParameters['url'] ?? '';
  }
  final address = Uri.tryParse(target);
  if (address == null || address.scheme != 'https' || address.host.isEmpty || target.length > 2048) {
    return null;
  }
  return target;
}
