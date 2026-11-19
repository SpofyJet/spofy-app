#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
XRAY=${SPOFY_XRAY_DIR:-$(cd "$ROOT/../spofy-xray-core" && pwd)}
CACHE=${SPOFY_GO_CACHE:-/var/cache/spofy-xray-build}
mkdir -p "$CACHE/gomod" "$CACHE/gocache"
exec docker run --rm --memory=${BUILD_MEM:-2500m} --cpus=${BUILD_CPUS:-2} ${DOCKER_EXTRA:-} \
  -e CGO_ENABLED=${CGO_ENABLED:-0} -e GOOS=${GOOS:-linux} -e GOARCH=${GOARCH:-amd64} -e GOFLAGS=-mod=mod -e GOTOOLCHAIN=local \
  -v "$ROOT:/src/spofy-app" -v "$XRAY:/src/spofy-xray-core:ro" \
  -v "$CACHE/gomod:/go/pkg/mod" -v "$CACHE/gocache:/root/.cache" \
  -w /src/spofy-app/core ${GO_IMAGE:-golang:1.27.3} nice "$@"
