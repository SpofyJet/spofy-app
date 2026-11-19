#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
SCRIPT=$1
shift
exec docker run --rm --cap-add NET_ADMIN --device /dev/net/tun --sysctl net.ipv4.conf.all.rp_filter=2 --memory=1g --cpus=1 \
  -v "$ROOT/dist/spofy-core-linux-amd64:/x/spofy-core:ro" -v "$ROOT/testdata/private/sub.json:/w/sub.json:ro" -v "$ROOT/tools/e2e:/e2e:ro" \
  --entrypoint timeout ${LAB_IMAGE:-spofy-lab:3} -s KILL ${LAB_LIMIT:-240} sh "/e2e/$SCRIPT.sh" "$@"
