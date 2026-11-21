import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/state/model.dart';
import 'package:spofy/ui/strings.dart';
import 'package:spofy/ui/welcome.dart';
import 'package:spofy/ui/widgets.dart';

import 'scenes.dart';

const link = 'https://sub.example.test/sub/TESTTOKEN0123456789';

String? clipboard;

void main() {
  setUpAll(loadAssets);

  setUp(() => clipboard = null);

  Future<void> paste(WidgetTester tester, String? text, String label) async {
    clipboard = text;
    await tester.tap(find.bySemanticsLabel(label));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));
  }

  Future<void> fail(WidgetTester tester, Scene scene, String code) async {
    scene.model
      ..failure = Failure(Task.subscribe, code, '')
      ..notifyListeners();
    await tester.pump(const Duration(milliseconds: 300));
  }

  final states = <String, Future<void> Function(WidgetTester, Scene)>{
    '': (tester, scene) async {},
    '_no_link': (tester, scene) => paste(tester, 'привет', S.pasteLink),
    '_adding': (tester, scene) => paste(tester, link, S.pasteLink),
    '_failed': (tester, scene) => fail(tester, scene, 'sub_status'),
    '_form': (tester, scene) => open(tester, S.manual),
    '_form_empty': (tester, scene) async {
      await open(tester, S.manual);
      await paste(tester, '', S.paste);
    },
    '_form_failed': (tester, scene) async {
      await open(tester, S.manual);
      await fail(tester, scene, 'sub_unreachable');
    },
  };

  Future<Scene> reach(
    WidgetTester tester,
    String state,
    Size size, {
    double scale = 1,
    Brightness brightness = Brightness.dark,
    List<Locale> locales = const [Locale('ru', 'RU')],
  }) async {
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
      return call.method == 'Clipboard.getData' && clipboard != null ? {'text': clipboard} : null;
    });
    addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, null));
    final scene = Scene(browse: (_) {});
    await show(tester, scene, size, textScale: scale, brightness: brightness, locales: locales);
    await states[state]!(tester, scene);
    return scene;
  }

  for (final brightness in Brightness.values) {
    final theme = brightness == Brightness.light ? '_light' : '';
    for (final state in states.keys) {
      testWidgets('welcome$state$theme on a phone', (tester) async {
        await reach(tester, state, phone, brightness: brightness);
        await golden(tester, 'welcome$state${theme}_360');
      });
    }
    testWidgets('welcome$theme on a desktop window', (tester) async {
      await reach(tester, '', desktop, brightness: brightness);
      await golden(tester, 'welcome${theme}_desktop');
    });
  }

  for (final state in ['', '_no_link', '_form_empty']) {
    testWidgets('welcome$state on a 320 px phone with 130 % text', (tester) async {
      await reach(tester, state, const Size(320, 568), scale: 1.3);
      await golden(tester, 'welcome${state}_320');
    });
  }

  testWidgets('welcome_no_link in English on a 320 px phone with 130 % text', (tester) async {
    addTearDown(() => S.use(Lang.ru));
    await reach(tester, '_no_link', const Size(320, 568), scale: 1.3, locales: const [Locale('en', 'US')]);
    await golden(tester, 'welcome_no_link_en_320');
  });

  testWidgets('no welcome text is cut or overflows, down to a 320 px phone with 130 % text, in both languages', (
    tester,
  ) async {
    addTearDown(() => S.use(Lang.ru));
    for (final locale in const [Locale('ru', 'RU'), Locale('en', 'US')]) {
      for (final (size, scale) in [
        (phone, 1.0),
        (const Size(320, 568), 1.3),
        (const Size(320, 480), 1.5),
        (desktop, 1.3),
      ]) {
        for (final state in states.keys) {
          await reach(tester, state, size, scale: scale, locales: [locale]);
          expect(tester.takeException(), isNull, reason: '$state at $size × $scale, $locale');
          for (final text in tester.renderObjectList<RenderParagraph>(find.byType(RichText))) {
            expect(text.didExceedMaxLines, isFalse, reason: '$state at $size × $scale: ${text.text.toPlainText()}');
          }
        }
      }
    }
  });

  for (final brightness in Brightness.values) {
    testWidgets('every welcome state, ${brightness.name}: tap targets, labels and text contrast', (tester) async {
      final semantics = tester.ensureSemantics();
      for (final state in states.keys) {
        await reach(tester, state, phone, brightness: brightness);
        await expectLater(tester, meetsGuideline(androidTapTargetGuideline));
        await expectLater(tester, meetsGuideline(iOSTapTargetGuideline));
        await expectLater(tester, meetsGuideline(labeledTapTargetGuideline));
        await expectLater(tester, meetsGuideline(textContrastGuideline));
      }
      semantics.dispose();
    });
  }

  testWidgets('on a desktop window the welcome is a centred column of ${Welcome.column.round()} px', (tester) async {
    await reach(tester, '', desktop);
    final button = tester.getRect(find.byType(PrimaryButton));
    expect(button.width, Welcome.column);
    expect(button.center.dx, desktop.width / 2);
  });

  testWidgets('the title and the buttons stay in place when a problem is shown', (tester) async {
    Rect title(WidgetTester tester) => tester.getRect(find.text(S.noSubscriptionTitle));
    Rect button(WidgetTester tester) => tester.getRect(find.byType(PrimaryButton));
    await reach(tester, '', phone);
    final (top, bottom) = (title(tester), button(tester));
    await paste(tester, 'привет', S.pasteLink);
    expect([title(tester), button(tester)], [top, bottom]);
    final scene = await reach(tester, '', phone);
    for (final code in ['sub_status', 'sub_unreachable', 'sub_format', 'sub_ended']) {
      await fail(tester, scene, code);
      expect([title(tester), button(tester)], [top, bottom], reason: code);
    }
  });

  testWidgets('the form says in place that the clipboard is empty and sends nothing', (tester) async {
    final scene = await reach(tester, '_form_empty', phone);
    expect(find.text(S.noLink), findsOneWidget);
    expect(scene.transport.sent, isEmpty);
    await paste(tester, ' $link\n', S.paste);
    expect(find.text(S.noLink), findsNothing);
    expect(scene.transport.sent, ['{"id":1,"op":"subscribe","url":"$link"}']);
  });

  testWidgets('typing by hand puts the cursor in the field; a link from another app does not', (tester) async {
    await reach(tester, '_form', phone);
    expect(tester.widget<EditableText>(find.byType(EditableText)).focusNode.hasFocus, isTrue);
    final scene = await reach(tester, '', phone);
    scene.model.offer('spofy://add/$link');
    await tester.pump();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(tester.widget<EditableText>(find.byType(EditableText)).focusNode.hasFocus, isFalse);
  });
}
