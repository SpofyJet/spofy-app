import 'package:flutter/widgets.dart';

const motionKey = 'motion';

abstract final class Motion {
  static const tap = Duration(milliseconds: 90);
  static const small = Duration(milliseconds: 140);
  static const medium = Duration(milliseconds: 200);
  static const large = Duration(milliseconds: 240);
  static const settle = Duration(milliseconds: 600);
  static const turn = Duration(milliseconds: 1100);

  static const Curve enter = Curves.easeOutCubic;
  static const Curve exit = Curves.easeInCubic;
  static const Curve move = Curves.easeInOutCubic;

  static bool still(BuildContext context) => MediaQuery.disableAnimationsOf(context);

  static Duration of(BuildContext context, Duration time) => still(context) ? Duration.zero : time;

  static bool get system => WidgetsBinding.instance.platformDispatcher.accessibilityFeatures.disableAnimations;
}

class MotionScope extends StatelessWidget {
  const MotionScope({super.key, required this.off, required this.child});

  final bool off;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final data = MediaQuery.of(context);
    return MediaQuery(
      data: data.copyWith(disableAnimations: data.disableAnimations || off),
      child: ScrollConfiguration(behavior: const _Scroll(), child: child),
    );
  }
}

class _Scroll extends ScrollBehavior {
  const _Scroll();

  @override
  Widget buildOverscrollIndicator(BuildContext context, Widget child, ScrollableDetails details) {
    if (Motion.still(context) || getPlatform(context) != TargetPlatform.android) {
      return child;
    }
    return StretchingOverscrollIndicator(
      axisDirection: details.direction,
      clipBehavior: details.decorationClipBehavior ?? Clip.hardEdge,
      child: child,
    );
  }
}

class Unfold extends StatefulWidget {
  const Unfold({super.key, required this.open, required this.child});

  final bool open;
  final Widget child;

  @override
  State<Unfold> createState() => _UnfoldState();
}

class _UnfoldState extends State<Unfold> with SingleTickerProviderStateMixin {
  late final _size = AnimationController(vsync: this, duration: Motion.large, value: widget.open ? 1 : 0)
    ..addStatusListener((status) {
      if (status.isDismissed) {
        setState(() {});
      }
    });
  late final _eased = CurvedAnimation(parent: _size, curve: Motion.move);

  @override
  void didUpdateWidget(Unfold old) {
    super.didUpdateWidget(old);
    if (old.open == widget.open) {
      return;
    }
    if (Motion.still(context)) {
      _size.value = widget.open ? 1 : 0;
    } else if (widget.open) {
      _size.forward();
    } else {
      _size.reverse();
    }
  }

  @override
  void dispose() {
    _eased.dispose();
    _size.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (_size.isDismissed) {
      return const SizedBox.shrink();
    }
    return SizeTransition(
      sizeFactor: _eased,
      alignment: Alignment.topCenter,
      child: FadeTransition(opacity: _eased, child: widget.child),
    );
  }
}

class Rise extends StatefulWidget {
  const Rise({super.key, required this.child});

  final Widget child;

  @override
  State<Rise> createState() => _RiseState();
}

class _RiseState extends State<Rise> with SingleTickerProviderStateMixin {
  late final _in = AnimationController(vsync: this, duration: Motion.medium);
  late final _eased = CurvedAnimation(parent: _in, curve: Motion.enter);
  late final _lift = Tween(begin: const Offset(0, 0.12), end: Offset.zero).animate(_eased);

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (Motion.still(context)) {
      _in.value = 1;
    } else if (_in.isDismissed) {
      _in.forward();
    }
  }

  @override
  void dispose() {
    _eased.dispose();
    _in.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return FadeTransition(
      opacity: _eased,
      child: SlideTransition(position: _lift, child: widget.child),
    );
  }
}
