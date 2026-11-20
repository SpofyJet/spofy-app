import 'dart:typed_data';

const appsKey = 'apps';

class InstalledApp {
  const InstalledApp(this.id, this.name);

  final String id;
  final String name;
}

abstract interface class AppList {
  Future<List<InstalledApp>> installed();

  Future<Uint8List?> icon(String id);
}

enum AppsMode { all, except, only }

class AppsChoice {
  const AppsChoice([this.mode = AppsMode.all, this.list = const {}]);

  factory AppsChoice.of(Object? setting) {
    if (setting is! Map) {
      return const AppsChoice();
    }
    final list = setting['list'];
    final mode = AppsMode.values.asNameMap()[setting['mode']] ?? AppsMode.all;
    return AppsChoice(mode, {...(list is List ? list.whereType<String>() : const <String>[])});
  }

  final AppsMode mode;
  final Set<String> list;

  bool get complete => mode != AppsMode.only || list.isNotEmpty;

  Object? get setting => list.isEmpty ? null : {'mode': mode.name, 'list': list.toList()..sort()};

  AppsChoice among(Iterable<String> known) => AppsChoice(mode, list.intersection(known.toSet()));
}
