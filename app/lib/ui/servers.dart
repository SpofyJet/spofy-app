import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/model.dart';
import 'icons.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

const serverSortKey = 'serverSort';
const sortSpeed = 'speed';
const sortName = 'name';

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

class Place {
  const Place(this.key, this.name, this.country, this.servers);

  final String key;
  final String name;
  final String country;
  final List<Server> servers;

  int get best {
    var best = 0;
    for (final s in servers) {
      if (s.measured && (best == 0 || s.delayMs < best)) {
        best = s.delayMs;
      }
    }
    return best;
  }

  int get tier => servers.map((s) => s.tier).reduce((a, b) => a < b ? a : b);
}

String placeName(Server s) {
  final stem = S.place(s).replaceFirst(RegExp(r'[\s#№\-_.]*\d+$'), '').trim();
  if (stem.isEmpty || stem.toUpperCase() == s.country.toUpperCase()) {
    return s.country.isEmpty ? S.place(s) : S.country(s.country);
  }
  return stem;
}

List<Place> places(List<Server> servers, {required String sort, String query = ''}) {
  final wanted = query.trim().toLowerCase();
  final groups = <String, List<Server>>{};
  final names = <String, String>{};
  for (final s in ordered(servers)) {
    final name = placeName(s);
    final key = '${s.country.toUpperCase()}|${name.toLowerCase()}';
    final found =
        wanted.isEmpty ||
        [name, s.country, S.country(s.country), S.place(s), S.via(s)].any((t) => t.toLowerCase().contains(wanted));
    if (found) {
      groups.putIfAbsent(key, () => []).add(s);
      names[key] = name;
    }
  }
  final list = [
    for (final e in groups.entries) Place(e.key, names[e.key]!, e.value.first.country, List.unmodifiable(e.value)),
  ];
  list.sort((a, b) {
    if (sort == sortName) {
      return a.name.toLowerCase().compareTo(b.name.toLowerCase());
    }
    if (a.tier != b.tier) {
      return a.tier.compareTo(b.tier);
    }
    final x = a.best, y = b.best;
    if ((x > 0) != (y > 0)) {
      return x > 0 ? -1 : 1;
    }
    return x != y ? x.compareTo(y) : a.name.compareTo(b.name);
  });
  return list;
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

class ServersScreen extends StatefulWidget {
  const ServersScreen({super.key, required this.model, required this.onDone});

  final AppModel model;
  final VoidCallback onDone;

  @override
  State<ServersScreen> createState() => _ServersScreenState();
}

class _ServersScreenState extends State<ServersScreen> {
  final _query = TextEditingController();
  final _open = <String>{};

  @override
  void initState() {
    super.initState();
    _query.addListener(() => setState(() {}));
  }

  @override
  void dispose() {
    _query.dispose();
    super.dispose();
  }

  void _pick(String server) {
    widget.model.choose(server);
    widget.onDone();
  }

  @override
  Widget build(BuildContext context) {
    final model = widget.model;
    return Backdrop(
      child: ListenableBuilder(
        listenable: model,
        builder: (context, _) {
          final connection = model.connection;
          final servers = model.snapshot.servers;
          final sort = model.snapshot.settings[serverSortKey] == sortName ? sortName : sortSpeed;
          final query = _query.text;
          final groups = places(servers, sort: sort, query: query);
          final failure = model.failure;
          final measuring = model.running(Task.measure);
          final subscription = model.snapshot.subscription;
          final notes = S.said(subscription?.notes ?? const []);
          final page = Uri.tryParse(subscription?.webPageUrl ?? '');
          final browse = model.browse;
          final target = noteLink(notes) ?? (page != null && page.isScheme('https') ? page.toString() : null);
          final rows = <Widget>[
            if (failure?.task == Task.measure) Notice(S.failure(failure!)),
            if (servers.length > 6) SearchField(controller: _query, hint: S.search, clear: S.appsClear),
            if (servers.length > 3)
              Segments<String>(
                options: const [(sortSpeed, S.sortSpeed), (sortName, S.sortName)],
                value: sort,
                onChanged: (value) => model.set(serverSortKey, value == sortSpeed ? null : value),
              ),
            if (query.isEmpty)
              Tile(
                leading: const CountryMark('A'),
                title: S.auto,
                subtitle: S.autoHint,
                selected: connection.auto,
                arrow: false,
                onTap: () => _pick(autoChoice),
              ),
            if (groups.isEmpty && query.isNotEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 24),
                child: Text(S.nothingFound, style: Ink.note, textAlign: TextAlign.center),
              ),
            for (final place in groups) ..._place(place, connection, query.isNotEmpty),
            if (notes.isNotEmpty)
              Tile(
                leading: const GlyphMark(Glyph.warning),
                title: notes.first,
                subtitle: notes.skip(1).join('\n'),
                wrap: true,
                onTap: browse == null || target == null ? null : () => browse(target),
              ),
          ];
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Header(
                title: S.servers,
                onBack: widget.onDone,
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
                  itemCount: rows.length,
                  separatorBuilder: (_, _) => const SizedBox(height: 8),
                  itemBuilder: (context, index) => rows[index],
                ),
              ),
            ],
          );
        },
      ),
    );
  }

  List<Widget> _place(Place place, Connection connection, bool searching) {
    if (place.servers.length == 1) {
      final s = place.servers.single;
      return [
        ServerTile(
          server: s,
          title: S.place(s),
          selected: connection.choice == s.id,
          active: connection.server == s.id,
          onTap: () => _pick(s.id),
        ),
      ];
    }
    final chosen = place.servers.any((s) => s.id == connection.choice);
    final open = searching || _open.contains(place.key) || (chosen && !_open.contains('-${place.key}'));
    return [
      PlaceTile(
        place: place,
        open: open,
        selected: chosen && !open,
        onTap: () => setState(() {
          if (open) {
            _open
              ..remove(place.key)
              ..add('-${place.key}');
          } else {
            _open
              ..add(place.key)
              ..remove('-${place.key}');
          }
        }),
      ),
      if (open)
        for (final s in place.servers)
          Padding(
            padding: const EdgeInsets.only(left: 20),
            child: ServerTile(
              server: s,
              title: S.place(s),
              selected: connection.choice == s.id,
              active: connection.server == s.id,
              mark: false,
              onTap: () => _pick(s.id),
            ),
          ),
    ];
  }
}

class ServerTile extends StatelessWidget {
  const ServerTile({
    super.key,
    required this.server,
    required this.title,
    required this.onTap,
    this.selected = false,
    this.active = false,
    this.mark = true,
  });

  final Server server;
  final String title;
  final VoidCallback onTap;
  final bool selected;
  final bool active;
  final bool mark;

  @override
  Widget build(BuildContext context) {
    final via = S.via(server);
    final delay = S.delay(server.delayMs);
    return Pressable(
      label: [title, via, delay].join(', '),
      onTap: onTap,
      child: Semantics(
        selected: selected,
        child: Panel(
          color: selected ? Color.alphaBlend(Ink.selected, Ink.surface) : Ink.surface,
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
          child: ConstrainedBox(
            constraints: const BoxConstraints(minHeight: 44),
            child: Row(
              children: [
                if (mark) ...[
                  CountryMark(server.country.isEmpty ? '—' : server.country, active: active),
                  const SizedBox(width: 12),
                ],
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                        via,
                        style: Ink.type(11, 600, selected ? Ink.accent : Ink.faint, spacing: 0.6),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                      const SizedBox(height: 2),
                      Text(title, style: Ink.strong, maxLines: 1, overflow: TextOverflow.ellipsis),
                    ],
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.only(left: 12),
                  child: Text(delay, style: Ink.note.copyWith(color: delayColor(server.delayMs))),
                ),
                if (selected)
                  const Padding(
                    padding: EdgeInsets.only(left: 10),
                    child: GlyphIcon(Glyph.check, color: Ink.accent, size: 20),
                  ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class PlaceTile extends StatelessWidget {
  const PlaceTile({super.key, required this.place, required this.open, required this.onTap, this.selected = false});

  final Place place;
  final bool open;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final best = place.best;
    final count = S.serversCount(place.servers.length);
    return Semantics(
      expanded: open,
      child: Pressable(
        label: [place.name, count, S.best(best)].where((p) => p.isNotEmpty).join(', '),
        onTap: onTap,
        child: Panel(
          color: selected ? Color.alphaBlend(Ink.selected, Ink.surface) : Ink.surface,
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
          child: ConstrainedBox(
            constraints: const BoxConstraints(minHeight: 40),
            child: Row(
              children: [
                CountryMark(place.country.isEmpty ? '—' : place.country),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(place.name, style: Ink.strong, maxLines: 1, overflow: TextOverflow.ellipsis),
                      const SizedBox(height: 2),
                      Text.rich(
                        TextSpan(
                          text: count,
                          children: [
                            if (best > 0) ...[
                              const TextSpan(text: ' · '),
                              TextSpan(
                                text: S.best(best),
                                style: TextStyle(color: delayColor(best)),
                              ),
                            ],
                          ],
                        ),
                        style: Ink.note,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ],
                  ),
                ),
                AnimatedRotation(
                  turns: open ? 0.25 : 0,
                  duration: const Duration(milliseconds: 160),
                  child: const GlyphIcon(Glyph.forward, color: Ink.faint, size: 20),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
