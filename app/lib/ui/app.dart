import 'dart:async';
import 'dart:ui' show AppExitResponse;

import 'package:flutter/widgets.dart';

import '../state/model.dart';
import 'about.dart';
import 'apps.dart';
import 'home.dart';
import 'logs.dart';
import 'servers.dart';
import 'settings.dart';
import 'strings.dart';
import 'subscription.dart';
import 'support.dart';
import 'theme.dart';
import 'update.dart';

class SpofyApp extends StatefulWidget {
  const SpofyApp({super.key, required this.model, this.now = DateTime.now});

  final AppModel model;
  final DateTime Function() now;

  @override
  State<SpofyApp> createState() => _SpofyAppState();
}

class _SpofyAppState extends State<SpofyApp> with WidgetsBindingObserver {
  final _navigator = GlobalKey<NavigatorState>();
  Timer? _poll;
  bool _visible = true;
  bool _subscription = false;
  bool _offering = false;
  int _offered = 0;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    widget.model.addListener(_schedule);
    _schedule();
  }

  @override
  void didUpdateWidget(SpofyApp old) {
    super.didUpdateWidget(old);
    if (old.model != widget.model) {
      old.model.removeListener(_schedule);
      widget.model.addListener(_schedule);
      _schedule();
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    widget.model.removeListener(_schedule);
    _poll?.cancel();
    super.dispose();
  }

  @override
  Future<AppExitResponse> didRequestAppExit() async {
    await widget.model.close();
    return AppExitResponse.exit;
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    final visible = state == AppLifecycleState.resumed;
    if (visible && !_visible) {
      widget.model.check();
      if (widget.model.subscription?.stopped ?? false) {
        widget.model.refresh();
      }
    }
    _visible = visible;
    _schedule();
  }

  void _schedule() {
    if (widget.model.ready && widget.model.offered != null && !_subscription) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _subscribe());
      WidgetsBinding.instance.scheduleFrame();
    }
    final update = widget.model.update;
    if (widget.model.ready && update != null && update.build != _offered && !_offering && shouldOffer(widget.model)) {
      _offered = update.build;
      WidgetsBinding.instance.addPostFrameCallback((_) => _offer());
      WidgetsBinding.instance.scheduleFrame();
    }
    final wanted = _visible && widget.model.connection.state.protecting;
    if (wanted && _poll == null) {
      widget.model.poll();
      _poll = Timer.periodic(const Duration(seconds: 2), (_) => widget.model.poll());
    } else if (!wanted && _poll != null) {
      _poll!.cancel();
      _poll = null;
    }
  }

  Future<void> _open(Widget Function(VoidCallback done) screen) {
    final navigator = _navigator.currentState!;
    return navigator.push(_route<void>((context) => screen(() => navigator.maybePop())));
  }

  void _subscribe() {
    if (_subscription || !mounted || _navigator.currentState == null) {
      return;
    }
    _subscription = true;
    final model = widget.model;
    _open((done) => SubscriptionScreen(model: model, now: widget.now, onDone: done))
        .whenComplete(() => _subscription = false);
  }

  void _offer() {
    final model = widget.model;
    final update = model.update;
    if (_offering || !mounted || _navigator.currentState == null || update == null) {
      return;
    }
    _offering = true;
    if (!update.required) {
      unawaited(model.set(updateShownKey, update.build));
    }
    _open((done) => UpdateScreen(model: model, onDone: done)).whenComplete(() => _offering = false);
  }

  void _settings() {
    final model = widget.model;
    _open(
      (done) => SettingsScreen(
        model: model,
        onDone: done,
        open: _open,
        onApps: () => _open((done) => AppsScreen(model: model, onDone: done)),
        onSupport: () => _open((done) => SupportScreen(model: model, onDone: done, now: widget.now)),
        onLogs: () => _open((done) => LogsScreen(model: model, onDone: done)),
        onAbout: () => _open((done) => AboutScreen(model: model, onDone: done, open: _open)),
      ),
    );
  }

  static PageRoute<T> _route<T>(WidgetBuilder builder, [RouteSettings? settings]) {
    return PageRouteBuilder<T>(
      settings: settings,
      transitionDuration: const Duration(milliseconds: 180),
      reverseTransitionDuration: const Duration(milliseconds: 140),
      pageBuilder: (context, _, _) => builder(context),
      transitionsBuilder: (context, animation, _, child) {
        final curved = CurvedAnimation(parent: animation, curve: Curves.easeOutCubic);
        return FadeTransition(
          opacity: curved,
          child: SlideTransition(
            position: Tween(begin: const Offset(0.04, 0), end: Offset.zero).animate(curved),
            child: child,
          ),
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final model = widget.model;
    return WidgetsApp(
      navigatorKey: _navigator,
      title: S.app,
      color: Ink.background,
      debugShowCheckedModeBanner: false,
      textStyle: Ink.body,
      pageRouteBuilder: <T>(RouteSettings settings, WidgetBuilder builder) => _route<T>(builder, settings),
      home: HomeScreen(
        model: model,
        now: widget.now,
        onServers: () => _open((done) => ServersScreen(model: model, onDone: done)),
        onSubscription: _subscribe,
        onSettings: _settings,
        onUpdate: () {
          if (!_offering) {
            _offering = true;
            _open((done) => UpdateScreen(model: model, onDone: done)).whenComplete(() => _offering = false);
          }
        },
      ),
    );
  }
}
