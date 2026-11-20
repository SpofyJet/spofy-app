#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
IN=$(cd "$1" && pwd)
mkdir -p "$2"
OUT=$(cd "$2" && pwd)
KEY=$(cd "${SPOFY_SIGN_KEY:?a folder with release.p12 and pass}" && pwd)
docker run --rm --network none --memory=1g -v "$IN:/in:ro" -v "$OUT:/out" -v "$KEY:/key:ro" -v "$ROOT/app/android/signer.sha256:/pin:ro" "$("$ROOT/tools/android/image.sh")" bash -c '
  set -euo pipefail
  for apk in /in/*-release.apk; do
    name=$(basename "$apk")
    java -jar /opt/apksigner.jar sign --ks /key/release.p12 --ks-pass file:/key/pass --min-sdk-version 24 --v4-signing-enabled false --out "/out/$name" "$apk"
    java -jar /opt/apksigner.jar verify --min-sdk-version 24 --print-certs "/out/$name" | sed -n "s/^.*certificate SHA-256 digest: //p" | sort -u | cmp -s - /pin || { echo "FAIL: $name is not signed by the pinned key"; rm -f "/out/$name"; exit 1; }
  done'
echo "SIGN OK"
