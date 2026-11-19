import 'dart:async';
import 'dart:convert';

import 'protocol.dart';

abstract interface class Transport {
  Stream<String> get lines;

  void send(String line);

  Future<void> close();
}

class CoreClosed implements Exception {
  const CoreClosed();

  @override
  String toString() => 'the core is not running';
}

class CoreClient {
  CoreClient(this._transport) {
    _lines = _transport.lines.listen(_receive, onError: _fault, onDone: _finish);
  }

  final Transport _transport;
  late final StreamSubscription<String> _lines;
  final _pending = <int, Completer<Reply>>{};
  final _events = StreamController<Event>.broadcast(sync: true);
  final _faults = StreamController<Object>.broadcast(sync: true);
  int _last = 0;
  bool _closed = false;

  Stream<Event> get events => _events.stream;

  Stream<Object> get faults => _faults.stream;

  bool get closed => _closed;

  Future<Reply> call(Request request) {
    if (_closed) {
      return Future.error(const CoreClosed());
    }
    final id = ++_last;
    final done = Completer<Reply>();
    _pending[id] = done;
    _transport.send(jsonEncode(request.toJson(id)));
    return done.future;
  }

  Future<void> close() async {
    if (_closed) {
      return;
    }
    await _transport.close();
    await _lines.cancel();
    _finish();
  }

  void _receive(String line) {
    if (line.trim().isEmpty) {
      return;
    }
    final Message message;
    try {
      message = Message.parse(line);
    } on ProtocolError catch (error) {
      _fault(error);
      return;
    }
    switch (message) {
      case Reply reply:
        _pending.remove(reply.re)?.complete(reply);
      case Event event:
        _events.add(event);
    }
  }

  void _fault(Object error, [StackTrace? _]) {
    if (!_faults.isClosed) {
      _faults.add(error);
    }
  }

  void _finish() {
    if (_closed) {
      return;
    }
    _closed = true;
    final waiting = _pending.values.toList();
    _pending.clear();
    for (final done in waiting) {
      done.completeError(const CoreClosed());
    }
    _events.close();
    _faults.close();
  }
}
