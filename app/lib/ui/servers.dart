import 'package:flutter/widgets.dart';

import '../core/protocol.dart';
import '../state/model.dart';
import 'icons.dart';
import 'motion.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

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

  bool get silent => servers.every((s) => s.silent);
}

String placeName(Server s) {
  final stem = S.place(s).replaceFirst(RegExp(r'[\s#№\-_.]*\d+$'), '').trim();
  if (stem.isEmpty || stem.toUpperCase() == s.country.toUpperCase()) {
    return s.country.isEmpty ? S.place(s) : S.country(s.country);
  }
  return stem;
}

List<Place> places(List<Server> servers, {String query = ''}) {
  final wanted = query.trim().toLowerCase();
  final groups = <String, List<Server>>{};
  final names = <String, String>{};
  for (final s in servers) {
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
  return [
    for (final e in groups.entries) Place(e.key, names[e.key]!, e.value.first.country, List.unmodifiable(e.value)),
  ];
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
          final query = _query.text;
          final groups = places(servers, query: query);
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
            if (servers.isEmpty) ...[
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 24, 16, 0),
                child: Balanced(S.noServers, style: Ink.note),
              ),
              QuietButton(label: S.refreshSubscription, busy: model.running(Task.refresh), onTap: model.refresh),
              if (failure?.task == Task.refresh) Notice(S.failure(failure!)),
            ] else if (query.isEmpty)
              Tile(
                leading: GlyphMark(Glyph.bolt, color: Ink.accent),
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
            for (final place in groups) _place(place, connection, query.isNotEmpty),
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
                  glyph: Glyph.speed,
                  label: measuring ? S.measuring : S.measure,
                  busy: measuring,
                  onTap: servers.isEmpty ? null : model.measure,
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

  Widget _place(Place place, Connection connection, bool searching) {
    if (place.servers.length == 1) {
      final s = place.servers.single;
      return ServerTile(
        key: ValueKey(place.key),
        server: s,
        title: S.place(s),
        selected: connection.choice == s.id,
        active: connection.server == s.id,
        onTap: () => _pick(s.id),
      );
    }
    final chosen = place.servers.any((s) => s.id == connection.choice);
    final open = searching || _open.contains(place.key) || (chosen && !_open.contains('-${place.key}'));
    return Column(
      key: ValueKey(place.key),
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
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
        Unfold(
          open: open,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final s in place.servers)
                Padding(
                  padding: const EdgeInsets.only(left: 20, top: 8),
                  child: ServerTile(
                    server: s,
                    title: S.place(s),
                    selected: connection.choice == s.id,
                    active: connection.server == s.id,
                    mark: false,
                    onTap: () => _pick(s.id),
                  ),
                ),
            ],
          ),
        ),
      ],
    );
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
    final delay = S.answer(server);
    final tone = server.silent ? Ink.warning : delayColor(server.delayMs);
    final tight = MediaQuery.textScalerOf(context).scale(100) > 115 || MediaQuery.sizeOf(context).width < 340;
    final line = Ink.type(12, 400, selected ? Ink.accent : Ink.faint);
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
                      Text(title, style: Ink.strong, maxLines: tight ? 2 : 1, overflow: TextOverflow.ellipsis),
                      const SizedBox(height: 2),
                      if (tight)
                        Folded([via, delay], style: line, last: tone)
                      else
                        Text(via, style: line, maxLines: 1, overflow: TextOverflow.ellipsis),
                    ],
                  ),
                ),
                if (!tight)
                  Padding(
                    padding: const EdgeInsets.only(left: 12),
                    child: Text(delay, style: Ink.note.copyWith(color: tone)),
                  ),
                if (selected)
                  Padding(
                    padding: const EdgeInsets.only(left: 10),
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
    final tight = MediaQuery.textScalerOf(context).scale(100) > 115 || MediaQuery.sizeOf(context).width < 340;
    return Semantics(
      expanded: open,
      child: Pressable(
        label: [place.name, count, if (best > 0) S.best(best), if (place.silent) S.silent].join(', '),
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
                      Text(place.name, style: Ink.strong, maxLines: tight ? 2 : 1, overflow: TextOverflow.ellipsis),
                      const SizedBox(height: 2),
                      if (tight)
                        Folded([
                          count,
                          if (best > 0) S.best(best) else if (place.silent) S.silent,
                        ], last: best > 0 ? delayColor(best) : (place.silent ? Ink.warning : null))
                      else
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
                              ] else if (place.silent) ...[
                                const TextSpan(text: ' · '),
                                TextSpan(
                                  text: S.silent,
                                  style: TextStyle(color: Ink.warning),
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
                  duration: Motion.of(context, Motion.small),
                  curve: Motion.move,
                  child: GlyphIcon(Glyph.forward, color: Ink.faint, size: 20),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
