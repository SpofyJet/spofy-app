#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
SPOFY_SHEETS=/src/app/build/sheets "$ROOT/tools/flutter.sh" flutter test --no-pub tool/sheets_test.dart --update-goldens --reporter=failures-only
ls "$ROOT/app/build/sheets"
