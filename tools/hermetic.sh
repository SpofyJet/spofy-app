#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
BIN=$(realpath "${1:-$ROOT/dist/spofy-core-linux-amd64}")
IMAGE=${HERMETIC_IMAGE:-spofy-hermetic:1}
docker image inspect "$IMAGE" >/dev/null 2>&1 || docker build -q -t "$IMAGE" "$ROOT/tools/hermetic" >/dev/null
exec docker run --rm --network none --cap-add NET_ADMIN --cap-add SYS_ADMIN --security-opt apparmor=unconfined --device /dev/net/tun --memory=1g --cpus=2 -e HERMETIC_ONLY -e HERMETIC_LOG \
  -v "$BIN:/x/spofy-core:ro" -v "$ROOT/tools/hermetic:/h:ro" \
  --entrypoint timeout "$IMAGE" -s KILL "${HERMETIC_LIMIT:-300}" sh -c 'mount -o remount,rw /proc/sys && python3 /h/stand.py /x/spofy-core'
