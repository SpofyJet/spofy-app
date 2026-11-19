#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
IMAGE=${FLUTTER_IMAGE:-spofy-flutter:3.50.0}
CACHE=${FLUTTER_CACHE:-/var/cache/spofy-flutter}
docker image inspect "$IMAGE" >/dev/null 2>&1 || docker build -q -t "$IMAGE" "$ROOT/tools/flutter" >/dev/null
mkdir -p "$CACHE"
exec docker run --rm --memory="${BUILD_MEM:-3g}" --cpus="${BUILD_CPUS:-2}" ${SPOFY_CORE:+-e SPOFY_CORE="$SPOFY_CORE"} ${SPOFY_SHEETS:+-e SPOFY_SHEETS="$SPOFY_SHEETS"} \
  -v "$ROOT:/src" -v "$CACHE:/cache" -w /src/app "$IMAGE" "$@"
