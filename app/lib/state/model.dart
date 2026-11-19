import 'dart:async';

import 'package:flutter/foundation.dart';

import '../core/client.dart';
import '../core/protocol.dart';

enum Task { subscribe, refresh, unsubscribe, connect, disconnect, measure }

class Failure {
  const Failure(this.task, this.code, this.error);

  final Task task;
  final String code;
  final String error;
}

class AppModel extends ChangeNotifier {
  AppModel(CoreClient client, {this.reopen, this.permit}) {
    _attach(client);
  }

  static const coreClosed = 'core_closed';
  static const coreProtocol = 'core_protocol';
  static const denied = 'vpn_denied';

  final Future<CoreClient> Function()? reopen;
  final Future<bool> Function()? permit;

  late CoreClient _client;
  StreamSubscription<Event>? _events;
  StreamSubscription<Object>? _faults;
  final _running = <Task>{};
  bool _disposed = false;
  bool _reviving = false;

  Snapshot snapshot = const Snapshot();
  Traffic traffic = const Traffic();
  bool ready = false;
  String fault = '';
  Failure? failure;

  Connection get connection => snapshot.connection;

  Subscription? get subscription => snapshot.subscription;

  bool running(Task task) => _running.contains(task);

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
    ready = true;
    _changed();
  }

  Future<bool> subscribe(String url) => _run(Task.subscribe, Request.subscribe(url.trim()));

  Future<bool> refresh() => _run(Task.refresh, const Request.refresh());

  Future<bool> unsubscribe() => _run(Task.unsubscribe, const Request.unsubscribe());

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
    if (open == null || _reviving || _disposed || !_client.closed) {
      return;
    }
    _reviving = true;
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

  Future<void> set(String key, Object? value) async {
    await _ask(Request.set(key, value));
  }

  Future<void> poll() async {
    final seen = (await _ask(const Request.traffic()))?.traffic;
    if (seen != null && (seen.up != traffic.up || seen.down != traffic.down)) {
      traffic = seen;
      _changed();
    }
  }

  void dismiss() {
    if (failure != null) {
      failure = null;
      _changed();
    }
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
    try {
      return await _client.call(request);
    } on CoreClosed {
      _lost();
      return null;
    }
  }

  void _apply(Event event) {
    snapshot = snapshot.apply(event);
    if (event is ConnectionEvent && !event.connection.state.protecting && (traffic.up != 0 || traffic.down != 0)) {
      traffic = const Traffic();
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
