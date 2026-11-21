import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/ui/icons.dart';
import 'package:spofy/ui/theme.dart';

import 'scenes.dart';

void main() {
  setUpAll(loadAssets);

  testWidgets('every glyph is drawn on the 24-unit grid and reads at 20 px in both palettes', (tester) async {
    tester.view.devicePixelRatio = 3;
    tester.view.physicalSize = const Size(360, 800) * 3;
    addTearDown(tester.view.reset);
    Widget sheet(Palette p) => ColoredBox(
      color: p.background,
      child: Padding(
        padding: const EdgeInsets.all(8),
        child: Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final glyph in Glyph.values)
              SizedBox(
                width: 56,
                child: Column(
                  children: [
                    DecoratedBox(
                      decoration: BoxDecoration(
                        color: p.raised,
                        borderRadius: const BorderRadius.all(Radius.circular(12)),
                      ),
                      child: SizedBox.square(
                        dimension: 40,
                        child: Center(child: GlyphIcon(glyph, color: p.muted, size: 20)),
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(glyph.name, style: Ink.type(10, 500, p.faint), maxLines: 1),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
    await tester.pumpWidget(
      Directionality(
        textDirection: TextDirection.ltr,
        child: Column(children: [sheet(Ink.dark), sheet(Ink.light)]),
      ),
    );
    for (final glyph in Glyph.values) {
      final size = tester.getSize(find.byWidgetPredicate((w) => w is GlyphIcon && w.glyph == glyph).first);
      expect(size, const Size(20, 20), reason: glyph.name);
    }
    await expectLater(find.byType(Column).first, matchesGoldenFile('goldens/glyphs_3x.png'));
  });
}
