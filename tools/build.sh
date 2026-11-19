#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
OS=${1:-linux}
ARCH=${2:-amd64}
EXT=""
[ "$OS" = windows ] && EXT=.exe
REV=$(git -C "$ROOT/../spofy-xray-core" rev-parse --short=7 HEAD)
[ "$(git -C "$ROOT/../spofy-xray-core" rev-parse HEAD)" = "$(cat "$ROOT/core/XRAY_REV")" ] || { echo "XRAY_REV does not match the fork checkout" >&2; exit 1; }
OUT=dist/spofy-core-$OS-$ARCH$EXT
mkdir -p "$ROOT/dist"
GOOS=$OS GOARCH=$ARCH "$ROOT/tools/go.sh" go build -o ../$OUT -trimpath -buildvcs=false \
  -ldflags="-X main.version=0.0.1+xray-$REV -X github.com/xtls/xray-core/core.build=spofy-$REV -s -w -buildid= -checklinkname=0" ./cmd/spofy-core
ls -l "$ROOT/$OUT" | awk '{print $5, $9}'
