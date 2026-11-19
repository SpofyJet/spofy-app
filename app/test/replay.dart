import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/core/client.dart';

import 'transcript.dart';

class ReplayTransport implements Transport {
  ReplayTransport(this.exchanges);

  final List<Exchange> exchanges;
  final _out = StreamController<String>();
  int position = 0;
  bool closed = false;

  @override
  Stream<String> get lines => _out.stream;

  Exchange get next => exchanges[position];

  void skip() => position++;

  void inject(String line) => _out.add(line);

  @override
  void send(String line) {
    final sent = jsonDecode(line) as Map<String, Object?>;
    final exchange = exchanges[position++];
    final expected = jsonDecode(exchange.sent) as Map<String, Object?>;
    final id = sent.remove('id');
    expected.remove('id');
    expect(sent, expected, reason: 'request ${position - 1}');
    for (final received in exchange.received) {
      final message = jsonDecode(received) as Map<String, Object?>;
      if (message.containsKey('re')) {
        message['re'] = id;
      }
      _out.add(jsonEncode(message));
    }
  }

  @override
  Future<void> close() async {
    closed = true;
    await _out.close();
  }
}
