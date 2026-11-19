#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT=${1:-sheets}
SPOFY_SHEETS=/src/app/build/$OUT FLUTTER_DOCKER_ARGS="-e SPOFY_SHEETS_PER=${SHEETS_PER:-} -e SPOFY_SHEETS_RATIO=${SHEETS_RATIO:-}" \
  "$ROOT/tools/flutter.sh" flutter test --no-pub tool/sheets_test.dart --update-goldens --reporter=failures-only
ls "$ROOT/app/build/$OUT"
