set -u
SERVER=${1:-'(DE)'}
get() { curl -s -o /dev/null -w '%{http_code}' -m "${2:-6}" "$1"; }
nft add table inet labm
nft add counter inet labm leaked
nft add chain inet labm out '{ type filter hook output priority 10; }'
nft add rule inet labm out oifname "eth0" meta skgid != 4711 counter name leaked
leaked() { nft list counter inet labm leaked | grep -o 'packets [0-9]*' | cut -d' ' -f2; }
probe() { python3 -c '
import socket,time
q=bytes.fromhex("123401000001000000000000037777770767737461746963 03636f6d0000010001".replace(" ",""))
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
while True:
    try: s.sendto(q,("1.1.1.1",53))
    except OSError: pass
    time.sleep(0.005)
' >/dev/null 2>&1 & echo $!; }
scenario() {
  NAME=$1; shift
  /x/spofy-core run -sub /w/sub.json -server "$SERVER" -lab-identity "$@" > /tmp/o 2>/tmp/e &
  RUN=$!
  for i in $(seq 1 600); do grep -q '"event":"\(started\|failed\)"' /tmp/o 2>/dev/null && break; sleep 0.005; done
  FIRST=$(get https://www.gstatic.com/generate_204)
  PROBE=$(probe)
  ( while :; do curl -s -o /dev/null -m 1 https://www.gstatic.com/generate_204; done ) >/dev/null 2>&1 & LOOP=$!
  sleep 0.5
  nft reset counter inet labm leaked >/dev/null
  for k in 1 2 3 4; do kill -9 $(pgrep -f 'spofy-core engine' | head -1); sleep 0.5; done
  L=$(leaked)
  kill $PROBE $LOOP 2>/dev/null; pkill curl 2>/dev/null; wait $PROBE $LOOP 2>/dev/null
  AFTER=$(get https://www.gstatic.com/generate_204)
  echo "$NAME: first=$FIRST kills=4 leaked_packets=$L after_kills=$AFTER restored_events=$(grep -c '"event":"restored"' /tmp/o) downtimes_ms=$(grep -o '"downtimeMs":[0-9]*' /tmp/o | cut -d: -f2 | tr '\n' ' ')"
  kill -TERM $RUN; wait $RUN
  sleep 0.2
  echo "$NAME: after_stop direct=$(get https://www.gstatic.com/generate_204) guard_table=$(nft list tables | grep -c spofy_guard) tun=$(ip link show spofy0 >/dev/null 2>&1 && echo PRESENT || echo gone)"
}
scenario no_guard
scenario guard -guard
OK=0
for r in 1 2 3 4 5 6 7 8 9 10; do
  /x/spofy-core run -sub /w/sub.json -server "$SERVER" -guard > /tmp/o 2>/tmp/e &
  RUN=$!
  for i in $(seq 1 600); do grep -q '"event":"started"' /tmp/o 2>/dev/null && break; sleep 0.005; done
  BLOCKED=$(nft list tables | grep -c spofy_guard)
  kill -9 $RUN; sleep 0.4
  T=$(nft list tables | grep -c spofy_guard); E=$(pgrep -fc 'spofy-core engine'); D=$(get https://www.gstatic.com/generate_204 4)
  [ "$BLOCKED" = 1 ] && [ "$T" = 0 ] && [ "$E" = 0 ] && [ "$D" = 204 ] && ! ip link show spofy0 >/dev/null 2>&1 && OK=$((OK+1)) || echo "round $r: guard_was_up=$BLOCKED table_left=$T engines_left=$E direct=$D"
done
echo "supervisor_killed_10_times: clean_and_online=$OK/10"
