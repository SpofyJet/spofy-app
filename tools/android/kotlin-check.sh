#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
IMAGE=spofy-kotlin:$(cat "$ROOT/tools/android/kotlin/Dockerfile" "$ROOT/tools/android/kotlin/fetch.py" | sha256sum | cut -c1-12)
docker image inspect "$IMAGE" >/dev/null 2>&1 || docker build -q -t "$IMAGE" "$ROOT/tools/android/kotlin" >/dev/null
REV=${FLUTTER_ENGINE:-$("$ROOT/tools/flutter.sh" bash -c 'cat "$(dirname "$(readlink -f "$(command -v flutter)")")/internal/engine.version"' | tail -1)}
CACHE=${KOTLIN_CACHE:-$HOME/.cache/spofy-kotlin}/$REV
mkdir -p "$CACHE"
docker run --rm --memory=1500m -v "$ROOT:/src:ro" -v "$CACHE:/deps" "$IMAGE" bash -c '
  set -euo pipefail
  [ -f /deps/embedding.jar ] || python3 /opt/fetch.py embedding "'"$REV"'" /deps
  mkdir -p /tmp/gen /tmp/out
  python3 /src/tools/android/kotlin/stubs.py /src/app/android/app/src/main/res /tmp/gen/Generated.kt
  kotlinc -nowarn -jvm-target 17 -no-stdlib -cp "/opt/android.jar:/opt/kotlinc/lib/kotlin-stdlib.jar:$(ls /deps/*.jar | tr "\n" ":")" -d /tmp/out /src/app/android/app/src/main/kotlin/app/spofy/*.kt /tmp/gen/Generated.kt'
echo "KOTLIN OK"
