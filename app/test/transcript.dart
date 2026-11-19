import 'dart:io';

class Exchange {
  Exchange(this.sent);

  final String sent;
  final received = <String>[];
}

class Transcript {
  Transcript(this.exchanges);

  factory Transcript.load([String path = '../proto/samples/session.jsonl']) {
    final exchanges = <Exchange>[];
    for (final line in File(path).readAsLinesSync()) {
      if (line.startsWith('> ')) {
        exchanges.add(Exchange(line.substring(2)));
      } else if (line.startsWith('< ')) {
        exchanges.last.received.add(line.substring(2));
      } else if (line.isNotEmpty) {
        throw FormatException('unexpected line in the samples: $line');
      }
    }
    return Transcript(exchanges);
  }

  final List<Exchange> exchanges;
}
