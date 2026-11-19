set -u
get() { curl -s -o /dev/null -w '%{http_code} %{time_total}' -m 10 "$1"; }
python3 - <<'PY' > /tmp/names
import json
for x in json.load(open('/w/sub.json')): print(x['remarks'])
PY
HOSTIP=$(curl -s -m 8 https://api.ipify.org)
ok=0; n=0
while IFS= read -r name; do
  n=$((n+1))
  /x/spofy-core run -sub /w/sub.json -server "$name" > /tmp/o 2>/tmp/e &
  RUN=$!
  for i in $(seq 1 600); do grep -q '"event":"\(started\|failed\)"' /tmp/o 2>/dev/null && break; sleep 0.005; done
  MS=$(grep -o '"ms":[0-9]*' /tmp/o | head -1 | cut -d: -f2)
  A=$(get https://www.gstatic.com/generate_204); B=$(get https://www.gstatic.com/generate_204)
  IP=$(curl -s -m 8 https://api.ipify.org)
  [ "${B%% *}" = 204 ] && [ -n "$IP" ] && [ "$IP" != "$HOSTIP" ] && ok=$((ok+1)) && R=ok || R=FAIL
  echo "$R | start ${MS:-?} ms | 1st ${A} | 2nd ${B} | $name"
  kill -TERM $RUN; wait $RUN 2>/dev/null
  [ $R = FAIL ] && { grep -iE 'failed|error' /tmp/o /tmp/e | head -3 | cut -c1-220; }
done < /tmp/names
echo "TOTAL $ok/$n"
