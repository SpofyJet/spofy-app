#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
IMAGE=${FLUTTER_IMAGE:-spofy-flutter:$(sed -n 's/^ARG FLUTTER_VERSION=//p' "$ROOT/tools/flutter/Dockerfile")}
CACHE=${FLUTTER_CACHE:-/var/cache/spofy-flutter}
RECIPE=$(sha256sum "$ROOT/tools/flutter/Dockerfile" | cut -c1-16)
[ "$(docker image inspect -f '{{index .Config.Labels "spofy.recipe"}}' "$IMAGE" 2>/dev/null)" = "$RECIPE" ] || docker build -q --label spofy.recipe="$RECIPE" -t "$IMAGE" "$ROOT/tools/flutter" >/dev/null
mkdir -p "$CACHE"
exec docker run --rm --memory="${BUILD_MEM:-3g}" --cpus="${BUILD_CPUS:-2}" ${SPOFY_CORE:+-e SPOFY_CORE="$SPOFY_CORE"} ${SPOFY_SHEETS:+-e SPOFY_SHEETS="$SPOFY_SHEETS"} \
  ${FLUTTER_DOCKER_ARGS:-} -v "$ROOT:/src" -v "$CACHE:/cache" -w /src/app "$IMAGE" "$@"
