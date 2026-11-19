set -u
for P in "$@"; do
  /x/spofy-core measure -sub /w/sub.json -parallel "$P" > /tmp/m.$P 2>/tmp/me.$P &
  RUN=$!
  PEAK=0; PROCS=0
  while kill -0 $RUN 2>/dev/null; do
    R=$(ps -C spofy-core -o rss= | awk '{s+=$1} END {print s+0}'); N=$(ps -C spofy-core -o pid= | wc -l)
    [ "$R" -gt "$PEAK" ] && PEAK=$R; [ "$N" -gt "$PROCS" ] && PROCS=$N
    sleep 0.03
  done
  wait $RUN
  SUM=$(grep '"event":"measured"' /tmp/m.$P)
  echo "parallel=$P exit=$? total_ms=$(echo "$SUM" | grep -o '"ms":[0-9]*' | cut -d: -f2) ok=$(echo "$SUM" | grep -o '"ok":[0-9]*' | cut -d: -f2)/$(echo "$SUM" | grep -o '"servers":[0-9]*' | cut -d: -f2) peak_rss_all_processes_mb=$((PEAK/1024)) peak_processes=$PROCS left_processes=$(ps -C spofy-core -o stat= | tr -d ' \n' | sed 's/^$/0/') stderr_lines=$(wc -l < /tmp/me.$P)"
done
LAST=/tmp/m.$P
echo "delays_ms: $(grep -v '"event"' $LAST | python3 -c '
import sys,json
rows=[json.loads(l) for l in sys.stdin]
print(" ".join("%s=%s" % (r["server"], r["ms"] if not r.get("code") else r["code"]) for r in sorted(rows, key=lambda r: (bool(r.get("code")), r["ms"]))))')"
grep -v '"event"' $LAST | grep '"code":"[a-z_]' | cut -c1-200 | head -5
