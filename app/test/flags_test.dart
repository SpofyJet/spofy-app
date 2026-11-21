import 'dart:io';

import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spofy/ui/about.dart';
import 'package:spofy/ui/flags.dart';
import 'package:spofy/ui/widgets.dart';

void main() {
  test('every flag code has its picture and every picture has its code', () {
    final files = {for (final file in Directory('assets/flags').listSync()) file.uri.pathSegments.last};
    expect(files, {for (final code in flagCodes) '$code.webp'});
    expect(flagCodes, containsAll(['de', 'nl', 'ru', 'us', 'gb', 'eu', 'xk']));
  });

  test('the licence of the flag set is shown with the other licences', () {
    expect(coreLicenses, contains(('flag-icons', flagLicense)));
    expect(flagLicense, contains('Copyright (c) 2013 Panayiotis Lipiridis'));
  });

  testWidgets('a country shows its flag in the same 40 px, any other mark keeps its letters', (tester) async {
    for (final (code, flag) in [('DE', true), ('nl', true), ('A', false), ('—', false), ('#', false), ('ZZ', false)]) {
      for (final active in [false, true]) {
        await tester.pumpWidget(
          Directionality(
            textDirection: TextDirection.ltr,
            child: Center(child: CountryMark(code, active: active)),
          ),
        );
        expect(find.byType(Text).evaluate().isEmpty, flag, reason: code);
        expect(tester.getSize(find.byType(CountryMark)), const Size.square(40), reason: code);
      }
    }
  });
}
