import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/model.dart';
import 'icons.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

List<Server> ordered(List<Server> servers) {
  final indexed = [for (var i = 0; i < servers.length; i++) (i, servers[i])];
  indexed.sort((a, b) {
    final (ai, x) = a;
    final (bi, y) = b;
    if (x.tier != y.tier) {
      return x.tier.compareTo(y.tier);
    }
    if (x.measured != y.measured) {
      return x.measured ? -1 : 1;
    }
    if (x.measured && x.delayMs != y.delayMs) {
      return x.delayMs.compareTo(y.delayMs);
    }
    return ai.compareTo(bi);
  });
  return [for (final (_, s) in indexed) s];
}

String? noteLink(List<String> lines) {
  for (final line in lines) {
    final handle = RegExp(r'(?:^|\s)@([A-Za-z0-9_]{5,32})\b').firstMatch(line);
    if (handle != null) {
      return 'https://t.me/${handle.group(1)}';
    }
    final address = RegExp(
      r'\b(?:https://)?((?:[a-z0-9-]+\.)+[a-z]{2,}(?:/\S*)?)',
      caseSensitive: false,
    ).firstMatch(line);
    if (address != null) {
      return 'https://${address.group(1)}';
    }
  }
  return null;
}

class ServersScreen extends StatelessWidget {
  const ServersScreen({super.key, required this.model, required this.onDone});

  final AppModel model;
  final VoidCallback onDone;

  void _pick(String server) {
    model.choose(server);
    onDone();
  }

  @override
  Widget build(BuildContext context) {
    return Backdrop(
      child: ListenableBuilder(
        listenable: model,
        builder: (context, _) {
          final connection = model.connection;
          final servers = ordered(model.snapshot.servers);
          final failure = model.failure;
          final measuring = model.running(Task.measure);
          final subscription = model.snapshot.subscription;
          final notes = S.said(subscription?.notes ?? const []);
          final page = Uri.tryParse(subscription?.webPageUrl ?? '');
          final browse = model.browse;
          final target = noteLink(notes) ?? (page != null && page.isScheme('https') ? page.toString() : null);
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Header(
                title: S.servers,
                onBack: onDone,
                action: RoundButton(
                  glyph: Glyph.refresh,
                  label: measuring ? S.measuring : S.measure,
                  busy: measuring,
                  onTap: model.measure,
                ),
              ),
              Expanded(
                child: ListView.separated(
                  padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 16),
                  itemCount: servers.length + 1 + (failure?.task == Task.measure ? 1 : 0) + (notes.isEmpty ? 0 : 1),
                  separatorBuilder: (_, _) => const SizedBox(height: 8),
                  itemBuilder: (context, index) {
                    var at = index;
                    if (failure?.task == Task.measure) {
                      if (at == 0) {
                        return Notice(S.failure(failure!));
                      }
                      at--;
                    }
                    if (at == 0) {
                      return Tile(
                        leading: const CountryMark('A'),
                        title: S.auto,
                        subtitle: S.autoHint,
                        selected: connection.auto,
                        arrow: false,
                        onTap: () => _pick(autoChoice),
                      );
                    }
                    if (at > servers.length) {
                      return Tile(
                        leading: const GlyphMark(Glyph.warning),
                        title: notes.first,
                        subtitle: notes.skip(1).join('\n'),
                        wrap: true,
                        onTap: browse == null || target == null ? null : () => browse(target),
                      );
                    }
                    final server = servers[at - 1];
                    return Tile(
                      leading: CountryMark(server.country.isEmpty ? '—' : server.country),
                      title: S.serverName(server),
                      trailing: S.delay(server.delayMs),
                      selected: connection.choice == server.id,
                      arrow: false,
                      onTap: () => _pick(server.id),
                    );
                  },
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}
