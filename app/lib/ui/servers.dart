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
                  itemCount: servers.length + 1 + (failure?.task == Task.measure ? 1 : 0),
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
