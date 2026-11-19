#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
[ -x "$ROOT/dist/spofy-core-linux-amd64" ] || { echo "FAIL: dist/spofy-core-linux-amd64 is missing, run tools/build.sh linux amd64"; exit 1; }
"$ROOT/tools/flutter.sh" sh -c 'flutter pub get --enforce-lockfile >/dev/null && flutter test --no-pub --reporter=failures-only test/protocol_test.dart >/dev/null'
FLUTTER_DOCKER_ARGS="--network none --cap-add NET_ADMIN --cap-add SYS_ADMIN --security-opt apparmor=unconfined --device /dev/net/tun -v $ROOT/dist/spofy-core-linux-amd64:/x/spofy-core:ro" \
  exec "$ROOT/tools/flutter.sh" timeout -s KILL "${HERMETIC_LIMIT:-420}" sh -c \
  'mount -o remount,rw /proc/sys && python3 /src/tools/hermetic/stand.py /x/spofy-core flutter test --no-pub --no-version-check --reporter=expanded stand/stand_test.dart'
