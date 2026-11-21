import 'dart:convert';

const protocolVersion = 1;
const autoChoice = 'auto';

class ProtocolError implements Exception {
  const ProtocolError(this.message);

  final String message;

  @override
  String toString() => message;
}

Map<String, Object?> _object(Object? value, String what, List<String> required, [List<String> optional = const []]) {
  if (value is! Map<String, Object?>) {
    throw ProtocolError('$what is not an object');
  }
  for (final key in value.keys) {
    if (!required.contains(key) && !optional.contains(key)) {
      throw ProtocolError('$what has an unknown field "$key"');
    }
  }
  for (final key in required) {
    if (!value.containsKey(key)) {
      throw ProtocolError('$what has no field "$key"');
    }
  }
  return value;
}

T _get<T>(Map<String, Object?> from, String key, String what) {
  final value = from[key];
  if (value is T) {
    return value;
  }
  throw ProtocolError('$what.$key has a wrong type');
}

enum VpnState {
  idle,
  measuring,
  connecting,
  connected,
  switching,
  stalled,
  offline,
  failed;

  static VpnState parse(String name) {
    for (final state in values) {
      if (state.name == name) {
        return state;
      }
    }
    throw ProtocolError('unknown connection state "$name"');
  }

  bool get busy => this == measuring || this == connecting || this == switching;

  bool get protecting => this == connected || this == switching || this == stalled || this == offline;
}

class Connection {
  const Connection({
    this.state = VpnState.idle,
    this.choice = autoChoice,
    this.server = '',
    this.delayMs = 0,
    this.code = '',
    this.error = '',
    this.since = 0,
  });

  factory Connection.fromJson(Object? json) {
    const what = 'connection';
    final m = _object(json, what, const ['state', 'choice', 'server', 'delayMs', 'code', 'error', 'since']);
    return Connection(
      state: VpnState.parse(_get<String>(m, 'state', what)),
      choice: _get<String>(m, 'choice', what),
      server: _get<String>(m, 'server', what),
      delayMs: _get<int>(m, 'delayMs', what),
      code: _get<String>(m, 'code', what),
      error: _get<String>(m, 'error', what),
      since: _get<int>(m, 'since', what),
    );
  }

  final VpnState state;
  final String choice;
  final String server;
  final int delayMs;
  final String code;
  final String error;
  final int since;

  bool get auto => choice == autoChoice;

  Map<String, Object?> toJson() => {
    'state': state.name,
    'choice': choice,
    'server': server,
    'delayMs': delayMs,
    'code': code,
    'error': error,
    'since': since,
  };
}

class Server {
  const Server({
    required this.id,
    required this.name,
    required this.label,
    required this.country,
    required this.protocol,
    required this.tier,
    required this.delayMs,
    required this.measuredAt,
    this.transport = '',
    this.security = '',
  });

  factory Server.fromJson(Object? json) {
    const what = 'server';
    final m = _object(json, what, const [
      'id',
      'name',
      'label',
      'country',
      'protocol',
      'transport',
      'security',
      'tier',
      'delayMs',
      'measuredAt',
    ]);
    return Server(
      id: _get<String>(m, 'id', what),
      name: _get<String>(m, 'name', what),
      label: _get<String>(m, 'label', what),
      country: _get<String>(m, 'country', what),
      protocol: _get<String>(m, 'protocol', what),
      transport: _get<String>(m, 'transport', what),
      security: _get<String>(m, 'security', what),
      tier: _get<int>(m, 'tier', what),
      delayMs: _get<int>(m, 'delayMs', what),
      measuredAt: _get<int>(m, 'measuredAt', what),
    );
  }

  final String id;
  final String name;
  final String label;
  final String country;
  final String protocol;
  final String transport;
  final String security;
  final int tier;
  final int delayMs;
  final int measuredAt;

  bool get measured => delayMs > 0;

  bool get silent => delayMs <= 0 && measuredAt > 0;

  Map<String, Object?> toJson() => {
    'id': id,
    'name': name,
    'label': label,
    'country': country,
    'protocol': protocol,
    'transport': transport,
    'security': security,
    'tier': tier,
    'delayMs': delayMs,
    'measuredAt': measuredAt,
  };
}

class Subscription {
  const Subscription({
    required this.title,
    required this.upload,
    required this.download,
    required this.total,
    required this.expire,
    required this.supportUrl,
    required this.webPageUrl,
    required this.announce,
    required this.fetchedAt,
    required this.updateEvery,
    this.stop = '',
    this.notes = const [],
  });

  factory Subscription.fromJson(Object? json) {
    const what = 'subscription';
    final m = _object(json, what, const [
      'title',
      'upload',
      'download',
      'total',
      'expire',
      'supportUrl',
      'webPageUrl',
      'announce',
      'fetchedAt',
      'updateEvery',
      'stop',
      'notes',
    ]);
    return Subscription(
      title: _get<String>(m, 'title', what),
      upload: _get<int>(m, 'upload', what),
      download: _get<int>(m, 'download', what),
      total: _get<int>(m, 'total', what),
      expire: _get<int>(m, 'expire', what),
      supportUrl: _get<String>(m, 'supportUrl', what),
      webPageUrl: _get<String>(m, 'webPageUrl', what),
      announce: _get<String>(m, 'announce', what),
      fetchedAt: _get<int>(m, 'fetchedAt', what),
      updateEvery: _get<int>(m, 'updateEvery', what),
      stop: _get<String>(m, 'stop', what),
      notes: List<String>.unmodifiable(
        _get<List<Object?>>(m, 'notes', what).map((n) => _get<String>({'': n}, '', '$what.notes')),
      ),
    );
  }

  final String title;
  final int upload;
  final int download;
  final int total;
  final int expire;
  final String supportUrl;
  final String webPageUrl;
  final String announce;
  final int fetchedAt;
  final int updateEvery;
  final String stop;
  final List<String> notes;

  bool get stopped => stop.isNotEmpty;

  int get used => upload + download;

  bool get limited => total > 0;

  Map<String, Object?> toJson() => {
    'title': title,
    'upload': upload,
    'download': download,
    'total': total,
    'expire': expire,
    'supportUrl': supportUrl,
    'webPageUrl': webPageUrl,
    'announce': announce,
    'fetchedAt': fetchedAt,
    'updateEvery': updateEvery,
    'stop': stop,
    'notes': notes,
  };
}

class Traffic {
  const Traffic({this.up = 0, this.down = 0});

  factory Traffic.fromJson(Object? json) {
    const what = 'traffic';
    final m = _object(json, what, const ['up', 'down']);
    return Traffic(up: _get<int>(m, 'up', what), down: _get<int>(m, 'down', what));
  }

  final int up;
  final int down;

  Map<String, Object?> toJson() => {'up': up, 'down': down};
}

class JournalEntry {
  const JournalEntry({required this.at, required this.state, this.server = '', this.code = ''});

  factory JournalEntry.fromJson(Object? json) {
    const what = 'journal entry';
    final m = _object(json, what, const ['at', 'state', 'server', 'code']);
    return JournalEntry(
      at: _get<int>(m, 'at', what),
      state: VpnState.parse(_get<String>(m, 'state', what)),
      server: _get<String>(m, 'server', what),
      code: _get<String>(m, 'code', what),
    );
  }

  final int at;
  final VpnState state;
  final String server;
  final String code;

  Map<String, Object?> toJson() => {'at': at, 'state': state.name, 'server': server, 'code': code};
}

class Report {
  const Report({
    this.os = '',
    this.osVersion = '',
    this.model = '',
    this.device = '',
    this.startedAt = 0,
    this.journal = const [],
  });

  factory Report.fromJson(Object? json) {
    const what = 'report';
    final m = _object(json, what, const ['os', 'osVersion', 'model', 'device', 'startedAt', 'journal']);
    return Report(
      os: _get<String>(m, 'os', what),
      osVersion: _get<String>(m, 'osVersion', what),
      model: _get<String>(m, 'model', what),
      device: _get<String>(m, 'device', what),
      startedAt: _get<int>(m, 'startedAt', what),
      journal: List<JournalEntry>.unmodifiable(_get<List<Object?>>(m, 'journal', what).map(JournalEntry.fromJson)),
    );
  }

  final String os;
  final String osVersion;
  final String model;
  final String device;
  final int startedAt;
  final List<JournalEntry> journal;

  Map<String, Object?> toJson() => {
    'os': os,
    'osVersion': osVersion,
    'model': model,
    'device': device,
    'startedAt': startedAt,
    'journal': [for (final e in journal) e.toJson()],
  };
}

class Brief {
  const Brief({
    required this.id,
    required this.title,
    required this.kind,
    required this.active,
    required this.servers,
    required this.used,
    required this.total,
    required this.expire,
    this.stop = '',
  });

  factory Brief.fromJson(Object? json) {
    const what = 'subscriptions item';
    final m = _object(json, what, const [
      'id',
      'title',
      'kind',
      'active',
      'servers',
      'used',
      'total',
      'expire',
      'stop',
    ]);
    return Brief(
      id: _get<String>(m, 'id', what),
      title: _get<String>(m, 'title', what),
      kind: _get<String>(m, 'kind', what),
      active: _get<bool>(m, 'active', what),
      servers: _get<int>(m, 'servers', what),
      used: _get<int>(m, 'used', what),
      total: _get<int>(m, 'total', what),
      expire: _get<int>(m, 'expire', what),
      stop: _get<String>(m, 'stop', what),
    );
  }

  final String id;
  final String title;
  final String kind;
  final bool active;
  final int servers;
  final int used;
  final int total;
  final int expire;
  final String stop;

  bool get links => kind == 'links';

  Map<String, Object?> toJson() => {
    'id': id,
    'title': title,
    'kind': kind,
    'active': active,
    'servers': servers,
    'used': used,
    'total': total,
    'expire': expire,
    'stop': stop,
  };
}

class Update {
  const Update({
    required this.version,
    required this.build,
    this.notes = '',
    this.required = false,
    this.size = 0,
    this.done = 0,
    this.file = '',
  });

  factory Update.fromJson(Object? json) {
    const what = 'update';
    final m = _object(json, what, const ['version', 'build', 'notes', 'required', 'size', 'done', 'file']);
    return Update(
      version: _get<String>(m, 'version', what),
      build: _get<int>(m, 'build', what),
      notes: _get<String>(m, 'notes', what),
      required: _get<bool>(m, 'required', what),
      size: _get<int>(m, 'size', what),
      done: _get<int>(m, 'done', what),
      file: _get<String>(m, 'file', what),
    );
  }

  final String version;
  final int build;
  final String notes;
  final bool required;
  final int size;
  final int done;
  final String file;

  bool get ready => file.isNotEmpty;

  double get share => size <= 0 ? 0 : (done / size).clamp(0, 1).toDouble();

  Map<String, Object?> toJson() => {
    'version': version,
    'build': build,
    'notes': notes,
    'required': required,
    'size': size,
    'done': done,
    'file': file,
  };
}

class LogLine {
  const LogLine({required this.at, required this.source, required this.text});

  factory LogLine.fromJson(Object? json) {
    const what = 'log line';
    final m = _object(json, what, const ['at', 'source', 'text']);
    return LogLine(
      at: _get<int>(m, 'at', what),
      source: _get<String>(m, 'source', what),
      text: _get<String>(m, 'text', what),
    );
  }

  final int at;
  final String source;
  final String text;

  Map<String, Object?> toJson() => {'at': at, 'source': source, 'text': text};
}

List<Brief> _briefs(Object? json) {
  if (json is! List<Object?>) {
    throw const ProtocolError('subscriptions is not a list');
  }
  return List<Brief>.unmodifiable(json.map(Brief.fromJson));
}

List<Server> _servers(Object? json) {
  if (json is! List<Object?>) {
    throw const ProtocolError('servers is not a list');
  }
  return List<Server>.unmodifiable(json.map(Server.fromJson));
}

Map<String, Object?> _settings(Object? json) {
  if (json is! Map<String, Object?>) {
    throw const ProtocolError('settings is not an object');
  }
  return Map<String, Object?>.unmodifiable(json);
}

class Snapshot {
  const Snapshot({
    this.version = '',
    this.protocol = protocolVersion,
    this.connection = const Connection(),
    this.subscription,
    this.subscriptions = const [],
    this.servers = const [],
    this.settings = const {},
    this.build = 0,
    this.engine = '',
    this.update,
  });

  factory Snapshot.fromJson(Object? json) {
    const what = 'snapshot';
    final m = _object(json, what, const [
      'version',
      'protocol',
      'connection',
      'subscription',
      'subscriptions',
      'servers',
      'settings',
      'build',
      'engine',
      'update',
    ]);
    return Snapshot(
      version: _get<String>(m, 'version', what),
      protocol: _get<int>(m, 'protocol', what),
      connection: Connection.fromJson(m['connection']),
      subscription: m['subscription'] == null ? null : Subscription.fromJson(m['subscription']),
      subscriptions: _briefs(m['subscriptions']),
      servers: _servers(m['servers']),
      settings: _settings(m['settings']),
      build: _get<int>(m, 'build', what),
      engine: _get<String>(m, 'engine', what),
      update: m['update'] == null ? null : Update.fromJson(m['update']),
    );
  }

  final String version;
  final int protocol;
  final Connection connection;
  final Subscription? subscription;
  final List<Brief> subscriptions;
  final List<Server> servers;
  final Map<String, Object?> settings;
  final int build;
  final String engine;
  final Update? update;

  Server? server(String id) {
    for (final s in servers) {
      if (s.id == id) {
        return s;
      }
    }
    return null;
  }

  Snapshot apply(Event event) => switch (event) {
    ConnectionEvent e => _copy(connection: e.connection),
    SubscriptionEvent e => Snapshot(
      version: version,
      protocol: protocol,
      connection: connection,
      subscription: e.subscription,
      subscriptions: subscriptions,
      servers: servers,
      settings: settings,
      build: build,
      engine: engine,
      update: update,
    ),
    UpdateEvent e => Snapshot(
      version: version,
      protocol: protocol,
      connection: connection,
      subscription: subscription,
      subscriptions: subscriptions,
      servers: servers,
      settings: settings,
      build: build,
      engine: engine,
      update: e.update,
    ),
    SubscriptionsEvent e => _copy(subscriptions: e.subscriptions),
    ServersEvent e => _copy(servers: e.servers),
    SettingsEvent e => _copy(settings: e.settings),
  };

  Snapshot _copy({
    Connection? connection,
    List<Brief>? subscriptions,
    List<Server>? servers,
    Map<String, Object?>? settings,
  }) => Snapshot(
    version: version,
    protocol: protocol,
    connection: connection ?? this.connection,
    subscription: subscription,
    subscriptions: subscriptions ?? this.subscriptions,
    servers: servers ?? this.servers,
    settings: settings ?? this.settings,
    build: build,
    engine: engine,
    update: update,
  );

  Map<String, Object?> toJson() => {
    'version': version,
    'protocol': protocol,
    'connection': connection.toJson(),
    'subscription': subscription?.toJson(),
    'subscriptions': [for (final b in subscriptions) b.toJson()],
    'servers': [for (final s in servers) s.toJson()],
    'settings': settings,
    'build': build,
    'engine': engine,
    'update': update?.toJson(),
  };
}

sealed class Message {
  const Message();

  static Message parse(String line) {
    final Object? json;
    try {
      json = jsonDecode(line);
    } on FormatException {
      throw const ProtocolError('the core sent a line that is not JSON');
    }
    if (json is! Map<String, Object?>) {
      throw const ProtocolError('the core sent a message that is not an object');
    }
    if (json.containsKey('re')) {
      return Reply.fromJson(json);
    }
    if (json.containsKey('ev')) {
      return Event.fromJson(json);
    }
    throw const ProtocolError('the core sent a message of an unknown kind');
  }

  Map<String, Object?> toJson();
}

class Reply extends Message {
  const Reply({
    required this.re,
    required this.ok,
    this.code = '',
    this.error = '',
    this.snapshot,
    this.changed,
    this.traffic,
    this.report,
    this.logs,
  });

  factory Reply.fromJson(Object? json) {
    const what = 'reply';
    final m = _object(
      json,
      what,
      const ['re', 'ok'],
      const ['code', 'error', 'snapshot', 'changed', 'traffic', 'report', 'logs'],
    );
    return Reply(
      re: _get<int>(m, 're', what),
      ok: _get<bool>(m, 'ok', what),
      code: _get<String?>(m, 'code', what) ?? '',
      error: _get<String?>(m, 'error', what) ?? '',
      snapshot: m['snapshot'] == null ? null : Snapshot.fromJson(m['snapshot']),
      changed: _get<bool?>(m, 'changed', what),
      traffic: m['traffic'] == null ? null : Traffic.fromJson(m['traffic']),
      report: m['report'] == null ? null : Report.fromJson(m['report']),
      logs: m['logs'] == null
          ? null
          : List<LogLine>.unmodifiable(_get<List<Object?>>(m, 'logs', what).map(LogLine.fromJson)),
    );
  }

  final int re;
  final bool ok;
  final String code;
  final String error;
  final Snapshot? snapshot;
  final bool? changed;
  final Traffic? traffic;
  final Report? report;
  final List<LogLine>? logs;

  @override
  Map<String, Object?> toJson() => {
    're': re,
    'ok': ok,
    if (code.isNotEmpty) 'code': code,
    if (error.isNotEmpty) 'error': error,
    if (snapshot != null) 'snapshot': snapshot!.toJson(),
    if (changed != null) 'changed': changed,
    if (traffic != null) 'traffic': traffic!.toJson(),
    if (report != null) 'report': report!.toJson(),
    if (logs != null) 'logs': [for (final l in logs!) l.toJson()],
  };
}

sealed class Event extends Message {
  const Event();

  factory Event.fromJson(Object? json) {
    if (json is! Map<String, Object?>) {
      throw const ProtocolError('event is not an object');
    }
    final kind = json['ev'];
    if (kind is! String) {
      throw const ProtocolError('event.ev has a wrong type');
    }
    final m = _object(json, 'event', ['ev', kind]);
    return switch (kind) {
      'connection' => ConnectionEvent(Connection.fromJson(m[kind])),
      'subscription' => SubscriptionEvent(m[kind] == null ? null : Subscription.fromJson(m[kind])),
      'subscriptions' => SubscriptionsEvent(_briefs(m[kind])),
      'update' => UpdateEvent(m[kind] == null ? null : Update.fromJson(m[kind])),
      'servers' => ServersEvent(_servers(m[kind])),
      'settings' => SettingsEvent(_settings(m[kind])),
      _ => throw ProtocolError('unknown event "$kind"'),
    };
  }
}

class ConnectionEvent extends Event {
  const ConnectionEvent(this.connection);

  final Connection connection;

  @override
  Map<String, Object?> toJson() => {'ev': 'connection', 'connection': connection.toJson()};
}

class SubscriptionEvent extends Event {
  const SubscriptionEvent(this.subscription);

  final Subscription? subscription;

  @override
  Map<String, Object?> toJson() => {'ev': 'subscription', 'subscription': subscription?.toJson()};
}

class UpdateEvent extends Event {
  const UpdateEvent(this.update);

  final Update? update;

  @override
  Map<String, Object?> toJson() => {'ev': 'update', 'update': update?.toJson()};
}

class SubscriptionsEvent extends Event {
  const SubscriptionsEvent(this.subscriptions);

  final List<Brief> subscriptions;

  @override
  Map<String, Object?> toJson() => {
    'ev': 'subscriptions',
    'subscriptions': [for (final b in subscriptions) b.toJson()],
  };
}

class ServersEvent extends Event {
  const ServersEvent(this.servers);

  final List<Server> servers;

  @override
  Map<String, Object?> toJson() => {
    'ev': 'servers',
    'servers': [for (final s in servers) s.toJson()],
  };
}

class SettingsEvent extends Event {
  const SettingsEvent(this.settings);

  final Map<String, Object?> settings;

  @override
  Map<String, Object?> toJson() => {'ev': 'settings', 'settings': settings};
}

class Request {
  const Request._(this.op, [this._fields = const {}]);

  const Request.hello() : this._('hello');

  Request.subscribe(String url) : this._('subscribe', {'url': url});

  const Request.refresh() : this._('refresh');

  Request.unsubscribe([String? subscription]) : this._('unsubscribe', {'subscription': ?subscription});

  Request.activate(String subscription) : this._('activate', {'subscription': subscription});

  Request.connect([String? server]) : this._('connect', {'server': ?server});

  const Request.disconnect() : this._('disconnect');

  const Request.check() : this._('check');

  Request.online({required bool online}) : this._('online', {'online': online});

  const Request.measure() : this._('measure');

  Request.set(String key, Object? value) : this._('set', {'key': key, 'value': value});

  const Request.traffic() : this._('traffic');

  const Request.report() : this._('report');

  const Request.logs() : this._('logs');

  const Request.clearLogs() : this._('clearLogs');

  const Request.checkUpdate() : this._('checkUpdate');

  const Request.fetchUpdate() : this._('fetchUpdate');

  const Request.installUpdate() : this._('installUpdate');

  final String op;
  final Map<String, Object?> _fields;

  Map<String, Object?> toJson(int id) => {'id': id, 'op': op, ..._fields};
}
