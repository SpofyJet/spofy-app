#!/bin/bash
set -euo pipefail
HERE=$(cd "$(dirname "$0")/kotlin" && pwd)
IMAGE=spofy-kotlin:$(cat "$HERE/Dockerfile" "$HERE/fetch.py" | sha256sum | cut -c1-12)
docker image inspect "$IMAGE" >/dev/null 2>&1 || docker build -q -t "$IMAGE" "$HERE" >/dev/null
echo "$IMAGE"
