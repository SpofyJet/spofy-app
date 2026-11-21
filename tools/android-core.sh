#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
NDK=${ANDROID_NDK_HOME:-${ANDROID_NDK_LATEST_HOME:-}}
BIN=$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin
[ -d "$BIN" ] || { echo "ANDROID_NDK_HOME does not point to an NDK" >&2; exit 1; }
REV=$(cut -c1-7 "$ROOT/core/XRAY_REV")
PUB=$(sed -n 's/^version: //p' "$ROOT/app/pubspec.yaml")
API=24
for abi in ${ANDROID_ABIS:-arm64-v8a armeabi-v7a x86_64}; do
  case $abi in
    arm64-v8a) arch=arm64 cc=aarch64-linux-android$API-clang ;;
    armeabi-v7a) arch=arm cc=armv7a-linux-androideabi$API-clang ;;
    x86_64) arch=amd64 cc=x86_64-linux-android$API-clang ;;
    *) echo "unknown ABI $abi" >&2; exit 1 ;;
  esac
  out=$ROOT/app/android/app/src/main/jniLibs/$abi/libspofycore.so
  mkdir -p "$(dirname "$out")"
  (cd "$ROOT/core" && CGO_ENABLED=1 GOOS=android GOARCH=$arch GOARM=7 CC=$BIN/$cc go build -o "$out" -trimpath -buildvcs=false \
    -ldflags="-X main.version=${PUB%+*} -X main.build=${PUB#*+} -X main.engineRev=xray-$REV -X github.com/xtls/xray-core/core.build=spofy-$REV -s -w -buildid= -checklinkname=0 -extldflags=-Wl,-z,max-page-size=16384,--pack-dyn-relocs=android" ./cmd/spofy-core)
  ls -l "$out" | awk '{print $5, $9}'
done
