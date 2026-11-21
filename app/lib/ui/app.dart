import 'dart:async';
import 'dart:ui' show AppExitResponse;

import 'package:flutter/gestures.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import '../state/model.dart';
import 'about.dart';
import 'apps.dart';
import 'home.dart';
import 'logs.dart';
import 'motion.dart';
import 'servers.dart';
import 'settings.dart';
import 'strings.dart';
import 'subscription.dart';
import 'support.dart';
import 'theme.dart';
import 'update.dart';
import 'widgets.dart';

class SpofyApp extends StatefulWidget {
  const SpofyApp({super.key, required this.model, this.now = DateTime.now, this.dress, this.hold = false});

  final AppModel model;
  final DateTime Function() now;
  final void Function(bool dark, Color background)? dress;
  final bool hold;

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
  Palette? _palette;
  Lang? _lang;
  bool _still = false;
  Palette? _dressed;
  Timer? _held;

  @override
  void initState() {
    super.initState();
    if (widget.hold) {
      WidgetsBinding.instance.deferFirstFrame();
      _held = Timer(const Duration(milliseconds: 300), _show);
    }
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
    _show();
    WidgetsBinding.instance.removeObserver(this);
    widget.model.removeListener(_schedule);
    _poll?.cancel();
    super.dispose();
  }

  @override
  void didChangePlatformBrightness() => _look(again: true);

  @override
  void didChangeLocales(List<Locale>? locales) => _look(again: true);

  void _show() {
    final held = _held;
    if (held != null) {
      _held = null;
      held.cancel();
      WidgetsBinding.instance.allowFirstFrame();
    }
  }

  void _look({bool again = false}) {
    final system = WidgetsBinding.instance.platformDispatcher;
    final settings = widget.model.snapshot.settings;
    final palette = paletteFor(settings[themeKey], system.platformBrightness);
    final lang = langFor(settings[languageKey], system.locales);
    final still = settings[motionKey] == false;
    if (still != _still) {
      _still = still;
      if (_palette != null) {
        setState(() {});
      }
    }
    final first = _palette == null;
    if (!again && palette == _palette && lang == _lang) {
      return;
    }
    _palette = palette;
    _lang = lang;
    Ink.use(palette);
    S.use(lang);
    if (palette != _dressed) {
      _dressed = palette;
      widget.dress?.call(palette.dark, palette.background);
    }
    if (!first) {
      void mark(Element element) {
        element.markNeedsBuild();
        element.visitChildren(mark);
      }

      mark(context as Element);
    }
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
    _look();
    if (widget.model.ready || widget.model.fault.isNotEmpty) {
      _show();
    }
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
      widget.model.rest();
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

  bool _back() {
    if (widget.model.pending != null) {
      widget.model.keep();
      return true;
    }
    final navigator = _navigator.currentState;
    if (navigator == null || !navigator.canPop()) {
      return false;
    }
    navigator.maybePop();
    return true;
  }

  PageRoute<T> _route<T>(WidgetBuilder builder, [RouteSettings? settings]) =>
      _Page<T>(builder, () => _still || Motion.system, settings: settings);

  @override
  Widget build(BuildContext context) {
    final model = widget.model;
    return AnnotatedRegion<SystemUiOverlayStyle>(
      value: _palette!.bars,
      child: WidgetsApp(
        navigatorKey: _navigator,
        title: S.app,
        color: Ink.background,
        locale: Locale(_lang!.name),
        supportedLocales: const [Locale('ru'), Locale('en')],
        debugShowCheckedModeBanner: false,
        textStyle: Ink.body,
        pageRouteBuilder: <T>(RouteSettings settings, WidgetBuilder builder) => _route<T>(builder, settings),
        builder: (context, child) => MotionScope(
          off: _still,
          child: Focus(
            canRequestFocus: false,
            skipTraversal: true,
            onKeyEvent: (node, event) =>
                event is KeyDownEvent && event.logicalKey == LogicalKeyboardKey.escape && _back()
                ? KeyEventResult.handled
                : KeyEventResult.ignored,
            child: Listener(
              onPointerDown: (event) {
                if (event.buttons & kBackMouseButton != 0) {
                  _back();
                }
              },
              child: Stack(
                children: [
                  child!,
                  ReconnectAsk(model: model),
                ],
              ),
            ),
          ),
        ),
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
      ),
    );
  }
}

class ReconnectAsk extends StatelessWidget {
  const ReconnectAsk({super.key, required this.model});

  final AppModel model;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: model,
      builder: (context, _) {
        if (model.pending == null) {
          return const SizedBox.shrink();
        }
        return Align(
          alignment: Alignment.bottomCenter,
          child: SafeArea(
            minimum: const EdgeInsets.all(Ink.gutter),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 600),
              child: Semantics(
                liveRegion: true,
                child: Rise(
                  child: Panel(
                    color: Ink.raised,
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Text(S.reconnectAsk, style: Ink.body),
                        const SizedBox(height: 8),
                        Row(
                          children: [
                            Expanded(
                              child: QuietButton(label: S.cancel, onTap: model.keep),
                            ),
                            Expanded(
                              child: QuietButton(label: S.apply, color: Ink.accent, onTap: () => model.apply()),
                            ),
                          ],
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
          ),
        );
      },
    );
  }
}

class _Page<T> extends PageRoute<T> {
  _Page(this._build, this._still, {super.settings});

  final WidgetBuilder _build;
  final bool Function() _still;

  @override
  Color? get barrierColor => null;

  @override
  String? get barrierLabel => null;

  @override
  bool get maintainState => true;

  @override
  Duration get transitionDuration => _still() ? Duration.zero : Motion.large;

  @override
  Duration get reverseTransitionDuration => _still() ? Duration.zero : Motion.medium;

  @override
  bool didPop(T? result) {
    controller?.reverseDuration = reverseTransitionDuration;
    return super.didPop(result);
  }

  @override
  Widget buildPage(BuildContext context, Animation<double> animation, Animation<double> secondaryAnimation) =>
      _build(context);

  @override
  Widget buildTransitions(
    BuildContext context,
    Animation<double> animation,
    Animation<double> secondaryAnimation,
    Widget child,
  ) {
    if (Motion.still(context)) {
      return child;
    }
    final eased = CurvedAnimation(parent: animation, curve: Motion.enter);
    return FadeTransition(
      opacity: eased,
      child: SlideTransition(
        position: Tween(begin: const Offset(0.04, 0), end: Offset.zero).animate(eased),
        child: child,
      ),
    );
  }
}
