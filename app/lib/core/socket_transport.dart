import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'client.dart';

class SocketTransport implements Transport {
  SocketTransport._(this._socket)
    : lines = _socket.cast<List<int>>().transform(utf8.decoder).transform(const LineSplitter());

  static Future<SocketTransport> connect(
    String path, {
    Duration limit = const Duration(seconds: 10),
    Duration step = const Duration(milliseconds: 50),
  }) async {
    final address = InternetAddress(path, type: InternetAddressType.unix);
    final watch = Stopwatch()..start();
    while (true) {
      try {
        return SocketTransport._(await Socket.connect(address, 0));
      } on SocketException {
        if (watch.elapsed >= limit) {
          rethrow;
        }
        await Future<void>.delayed(step);
      }
    }
  }

  final Socket _socket;

  @override
  final Stream<String> lines;

  @override
  void send(String line) {
    try {
      _socket.writeln(line);
    } on Object {
      _socket.destroy();
    }
  }

  @override
  Future<void> close() async {
    _socket.destroy();
  }
}
