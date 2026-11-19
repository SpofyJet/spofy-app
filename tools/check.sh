#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
FORK="$ROOT/../spofy-xray-core"
[ "$(git -C "$FORK" rev-parse HEAD)" = "$(cat "$ROOT/core/XRAY_REV")" ] || { echo "FAIL: XRAY_REV differs from the fork checkout"; exit 1; }
for mod in $(grep -E '^replace .* => \./' "$FORK/go.mod" | awk '{print $2}'); do
  grep -q "^replace $mod => ../../spofy-xray-core/" "$ROOT/core/go.mod" || { echo "FAIL: replace for $mod is not mirrored in core/go.mod"; exit 1; }
done
cd "$ROOT"
if git ls-files -co --exclude-standard | grep -E '^(testdata/private|docs/research|dist|spike)/' | head -3 | grep .; then echo "FAIL: private files are tracked"; exit 1; fi
if git ls-files -co --exclude-standard -z | xargs -0 grep -lIE 'gh[pousr]_[A-Za-z0-9]{20}|github_pat_[A-Za-z0-9_]{20}|PRIVATE KEY-{5}' | head -3 | grep .; then echo "FAIL: secret in a tracked file"; exit 1; fi
if [ -f testdata/private/sub.json ]; then
  T=$(mktemp -d)
  git ls-files -co --exclude-standard -z | xargs -0 python3 tools/mkfixture.py testdata/private "$T" >/dev/null || { echo "FAIL: real subscription values in tracked files"; exit 1; }
  cmp -s "$T/sub.json" core/testdata/sub/sub.json && cmp -s "$T/headers.json" core/testdata/sub/headers.json || { echo "FAIL: core/testdata/sub differs from tools/mkfixture.py output"; exit 1; }
  rm -rf "$T"
fi
if grep -rnE '^\s*//[^g]' "$ROOT/core" --include='*.go' | grep -v '//go:' | head -5 | grep .; then echo "FAIL: comments in code"; exit 1; fi
CGO_ENABLED=1 BUILD_MEM=3g "$ROOT/tools/go.sh" sh -c 'test -z "$(gofmt -l .)" || { echo "FAIL: gofmt"; gofmt -l .; exit 1; }; go vet ./... && go test -race -count=1 ./...'
"$ROOT/tools/build.sh" linux amd64 >/dev/null
"$ROOT/tools/hermetic.sh" | tail -1 | grep -q "^HERMETIC OK" || { echo "FAIL: tools/hermetic.sh"; exit 1; }
"$ROOT/tools/app-check.sh" | tail -1 | grep -q "^APP OK" || { echo "FAIL: tools/app-check.sh"; exit 1; }
"$ROOT/tools/app-stand.sh" | tail -1 | grep -q "^HERMETIC OK" || { echo "FAIL: tools/app-stand.sh"; exit 1; }
"$ROOT/tools/android/kotlin-check.sh" | tail -1 | grep -q "^KOTLIN OK" || { "$ROOT/tools/android/kotlin-check.sh" || true; echo "FAIL: tools/android/kotlin-check.sh"; exit 1; }
echo "CHECK OK"
