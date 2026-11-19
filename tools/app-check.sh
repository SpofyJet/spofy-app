#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
[ -x "$ROOT/dist/spofy-core-linux-amd64" ] || { echo "FAIL: dist/spofy-core-linux-amd64 is missing, run tools/build.sh linux amd64"; exit 1; }
if grep -rnE '^\s*//' "$ROOT/app/lib" "$ROOT/app/test" "$ROOT/app/tool" --include='*.dart' | head -5 | grep .; then echo "FAIL: comments in code"; exit 1; fi
SPOFY_CORE=/src/dist/spofy-core-linux-amd64 "$ROOT/tools/flutter.sh" bash -c '
  set -euo pipefail
  flutter pub get --enforce-lockfile >/dev/null
  dart format --output=none --set-exit-if-changed lib test tool >/dev/null || { echo "FAIL: dart format"; exit 1; }
  flutter analyze --no-pub >/dev/null || { flutter analyze --no-pub || true; echo "FAIL: flutter analyze"; exit 1; }
  flutter test --no-pub --reporter=failures-only || { echo "FAIL: flutter test"; exit 1; }'
echo "APP OK"
