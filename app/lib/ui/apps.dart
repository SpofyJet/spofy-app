import 'dart:async';

import 'package:flutter/services.dart' show TextInputAction;
import 'package:flutter/widgets.dart';

import '../state/apps.dart';
import '../state/model.dart';
import 'icons.dart';
import 'strings.dart';
import 'theme.dart';
import 'widgets.dart';

class AppsScreen extends StatefulWidget {
  const AppsScreen({super.key, required this.model, required this.onDone});

  final AppModel model;
  final VoidCallback onDone;

  @override
  State<AppsScreen> createState() => _AppsScreenState();
}

class _AppsScreenState extends State<AppsScreen> {
  final _query = TextEditingController();
  final _focus = FocusNode();
  final _field = GlobalKey<EditableTextState>();
  final _icons = <String, Future<MemoryImage?>>{};
  late AppsMode _mode;
  late final Set<String> _marked;
  List<InstalledApp>? _apps;
  bool _touched = false;
  bool _locked = false;
  Timer? _later;

  @override
  void initState() {
    super.initState();
    final choice = widget.model.chosenApps;
    _mode = choice.mode;
    _marked = {...choice.list};
    _query.addListener(_typed);
    unawaited(_load({...choice.list}));
  }

  void _typed() => setState(() {});

  Future<void> _load(Set<String> first) async {
    var found = const <InstalledApp>[];
    var locked = false;
    try {
      found = await widget.model.apps?.installed() ?? found;
      locked = await widget.model.apps?.locked() ?? false;
    } on Exception {
      found = const [];
    }
    if (!mounted) {
      return;
    }
    setState(() {
      _locked = locked;
      _apps = [
        for (final app in found)
          if (first.contains(app.id)) app,
        for (final app in found)
          if (!first.contains(app.id)) app,
      ];
    });
  }

  Future<MemoryImage?> _icon(String id) async {
    try {
      final bytes = await widget.model.apps?.icon(id);
      return bytes == null || bytes.isEmpty ? null : MemoryImage(bytes);
    } on Exception {
      return null;
    }
  }

  @override
  void dispose() {
    _later?.cancel();
    _save();
    for (final icon in _icons.values) {
      unawaited(icon.then((image) => image?.evict()));
    }
    _query.dispose();
    _focus.dispose();
    super.dispose();
  }

  void _change(VoidCallback change) {
    setState(change);
    _touched = true;
    _later?.cancel();
    _later = Timer(const Duration(milliseconds: 700), _save);
  }

  void _save() {
    final apps = _apps ?? const [];
    var choice = AppsChoice(_mode, _marked);
    if (apps.isNotEmpty) {
      choice = choice.among(apps.map((app) => app.id));
    }
    if (!_touched || !choice.complete) {
      return;
    }
    _touched = false;
    unawaited(widget.model.set(appsKey, choice.setting));
  }

  Widget _search() {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      excludeFromSemantics: true,
      onTap: () => _field.currentState?.requestKeyboard(),
      child: Panel(
        padding: const EdgeInsets.fromLTRB(16, 4, 4, 4),
        child: Row(
          children: [
            Expanded(
              child: Stack(
                children: [
                  if (_query.text.isEmpty) Text(S.appsSearch, style: Ink.body.copyWith(color: Ink.faint)),
                  Semantics(
                    textField: true,
                    label: S.appsSearch,
                    child: EditableText(
                      key: _field,
                      controller: _query,
                      focusNode: _focus,
                      style: Ink.body,
                      cursorColor: Ink.accent,
                      backgroundCursorColor: Ink.faint,
                      selectionColor: Ink.selected,
                      textInputAction: TextInputAction.search,
                      autocorrect: false,
                      enableSuggestions: false,
                      maxLines: 1,
                    ),
                  ),
                ],
              ),
            ),
            if (_query.text.isEmpty)
              const SizedBox(height: 44)
            else
              RoundButton(glyph: Glyph.close, label: S.appsClear, onTap: _query.clear),
          ],
        ),
      ),
    );
  }

  Widget _list(List<InstalledApp>? apps) {
    if (apps == null) {
      return const Align(
        alignment: Alignment(0, -0.6),
        child: Spinner(size: 28, color: Ink.accent),
      );
    }
    final text = _query.text.trim().toLowerCase();
    final shown = text.isEmpty
        ? apps
        : [
            for (final app in apps)
              if (app.name.toLowerCase().contains(text) || app.id.toLowerCase().contains(text)) app,
          ];
    final notes = [
      if (_locked) const Notice(S.appsLocked),
      if (apps.isEmpty)
        const Notice(S.appsFailed)
      else if (_mode == AppsMode.only && _marked.isEmpty)
        const Notice(S.appsOnlyEmpty),
      if (apps.isNotEmpty && shown.isEmpty)
        Padding(
          padding: const EdgeInsets.only(top: 24),
          child: Text(S.appsNone, style: Ink.note, textAlign: TextAlign.center),
        ),
    ];
    return ListView.separated(
      keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
      padding: EdgeInsets.fromLTRB(Ink.gutter, 0, Ink.gutter, 16 + MediaQuery.viewInsetsOf(context).bottom),
      itemCount: notes.length + shown.length,
      separatorBuilder: (_, _) => const SizedBox(height: 8),
      itemBuilder: (context, index) {
        if (index < notes.length) {
          return notes[index];
        }
        final app = shown[index - notes.length];
        final marked = _marked.contains(app.id);
        return Semantics(
          checked: marked,
          child: Tile(
            leading: _Icon(_icons[app.id] ??= _icon(app.id)),
            title: app.name,
            selected: marked,
            arrow: false,
            wrap: true,
            onTap: () => _change(() => marked ? _marked.remove(app.id) : _marked.add(app.id)),
          ),
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    return Backdrop(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Header(title: S.apps, onBack: widget.onDone),
          Padding(
            padding: const EdgeInsets.fromLTRB(Ink.gutter, 4, Ink.gutter, 0),
            child: Segments<AppsMode>(
              value: _mode,
              options: const [(AppsMode.all, S.appsAll), (AppsMode.except, S.appsExcept), (AppsMode.only, S.appsOnly)],
              onChanged: (mode) => _change(() => _mode = mode),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(Ink.gutter + 4, 10, Ink.gutter + 4, 12),
            child: Text(S.appsHint(_mode), style: Ink.note),
          ),
          if (_mode != AppsMode.all) ...[
            Padding(padding: const EdgeInsets.fromLTRB(Ink.gutter, 0, Ink.gutter, 8), child: _search()),
            Expanded(child: _list(_apps)),
          ],
        ],
      ),
    );
  }
}

class _Icon extends StatelessWidget {
  const _Icon(this.image);

  final Future<MemoryImage?> image;

  static const _blank = DecoratedBox(
    decoration: BoxDecoration(color: Ink.raised, borderRadius: BorderRadius.all(Radius.circular(10))),
  );

  @override
  Widget build(BuildContext context) {
    return SizedBox.square(
      dimension: 40,
      child: FutureBuilder<MemoryImage?>(
        future: image,
        builder: (context, loaded) {
          final image = loaded.data;
          if (image == null) {
            return _blank;
          }
          return Image(
            image: image,
            width: 40,
            height: 40,
            gaplessPlayback: true,
            excludeFromSemantics: true,
            filterQuality: FilterQuality.medium,
            errorBuilder: (_, _, _) => _blank,
          );
        },
      ),
    );
  }
}
