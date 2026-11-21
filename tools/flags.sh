#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
IMAGE=alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
VERSION=7.6.0
INTEGRITY=sha512-hir/PsXwr0EESyizKrXJzmkzpLIMf+1vlNKLqAfvdh+bsU94a7LYWTiubkHKqptGoCPzOUGcm6c4PfC9/BaGSQ==
SIZE=102
NEAR=40
docker run --rm -i --memory=512m --cpus=2 -e VERSION=$VERSION -e INTEGRITY=$INTEGRITY -e SIZE=$SIZE -e NEAR=$NEAR \
  -e OWNER="$(id -u):$(id -g)" -v "$ROOT/app:/app" "$IMAGE" sh -eu <<'EOF'
apk add -q --no-cache rsvg-convert=2.62.3-r0 libwebp-tools=1.6.0-r0 >/dev/null
mkdir /tmp/download /tmp/source /tmp/png /tmp/flags
wget -q -O /tmp/download/flag-icons.tgz "https://registry.npmjs.org/flag-icons/-/flag-icons-$VERSION.tgz"
[ "sha512-$(sha512sum /tmp/download/flag-icons.tgz | cut -c1-128 | xxd -r -p | base64 -w0)" = "$INTEGRITY" ] \
  || { echo "FAIL: flag-icons $VERSION does not match its registry hash"; exit 1; }
tar -xzof /tmp/download/flag-icons.tgz -C /tmp/source
cd /tmp/source/package/flags/1x1
for file in ??.svg sh-ac.svg sh-ta.svg; do
  code=${file%.svg}
  code=${code#sh-}
  case $code in xx | pc) continue ;; esac
  rsvg-convert -w $((SIZE * 8)) -h $((SIZE * 8)) -o "/tmp/png/$code.png" "$file"
  cwebp -quiet -resize "$SIZE" "$SIZE" -near_lossless "$NEAR" -z 9 -metadata none "/tmp/png/$code.png" -o "/tmp/flags/$code.webp"
done
{
  printf "const flagLicense = '''\nflag-icons %s\nhttps://github.com/lipis/flag-icons\n\n" "$VERSION"
  awk 'BEGIN { RS = "" } { sub(/\n+$/, ""); gsub(/\n/, " "); printf "%s%s", (NR > 1 ? "\n\n" : ""), $0 }' ../../LICENSE
  printf "''';\n\nconst flagCodes = {\n"
  ls /tmp/flags | sed "s/\.webp\$//; s/.*/  '&',/"
  printf "};\n"
} >/tmp/flags.dart
rm -rf /app/assets/flags
mkdir -p /app/assets
cp -r /tmp/flags /app/assets/flags
cp /tmp/flags.dart /app/lib/ui/flags.dart
chown -R "$OWNER" /app/assets /app/lib/ui/flags.dart
echo "FLAGS OK: $(ls /tmp/flags | wc -l) flags, $(cat /tmp/flags/* | wc -c) bytes"
EOF
