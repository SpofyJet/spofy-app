import 'dart:async';

import 'package:flutter/widgets.dart';

import '../state/model.dart';
import 'home.dart';
import 'servers.dart';
import 'strings.dart';
import 'subscription.dart';
import 'theme.dart';

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
  void didChangeAppLifecycleState(AppLifecycleState state) {
    final visible = state == AppLifecycleState.resumed;
    if (visible && !_visible) {
      widget.model.check();
    }
    _visible = visible;
    _schedule();
  }

  void _schedule() {
    final wanted = _visible && widget.model.connection.state.protecting;
    if (wanted && _poll == null) {
      widget.model.poll();
      _poll = Timer.periodic(const Duration(seconds: 2), (_) => widget.model.poll());
    } else if (!wanted && _poll != null) {
      _poll!.cancel();
      _poll = null;
    }
  }

  void _open(Widget Function(VoidCallback done) screen) {
    final navigator = _navigator.currentState!;
    navigator.push(_route<void>((context) => screen(() => navigator.maybePop())));
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
        onSubscription: () => _open((done) => SubscriptionScreen(model: model, now: widget.now, onDone: done)),
      ),
    );
  }
}
