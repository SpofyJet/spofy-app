set -u
get() { curl -s -o /dev/null -w '%{http_code}' -m "${2:-6}" "$1"; }
now() { date +%s%N; }
ms() { echo $(( ($2 - $1) / 1000000 )); }
addr() { python3 -c '
import json,sys
want=sys.argv[1]
for c in json.load(open("/w/sub.json")):
    if "".join(ch.lower() for ch in c.get("remarks","") if ch.isalnum()) != want: continue
    s=c["outbounds"][0].get("settings",{})
    for k in ("vnext","servers"):
        if s.get(k): print(s[k][0]["address"]); sys.exit()
    print(s.get("address",""))
' "$1"; }
nft add table inet labm
nft add counter inet labm leaked
nft add chain inet labm out '{ type filter hook output priority 10; }'
nft add rule inet labm out oifname "eth0" meta skgid != 4711 counter name leaked
nft add chain inet labm cut '{ type filter hook output priority 20; }'
leaked() { nft list counter inet labm leaked | grep -o 'packets [0-9]*' | cut -d' ' -f2; }
state() { grep '"state":"connected"' /tmp/o | tail -1 | grep -o '"server":"[^"]*"' | cut -d'"' -f4; }
T0=$(now)
/x/spofy-core run -sub /w/sub.json -server auto -guard -check 2s "$@" > /tmp/o 2>/tmp/e &
RUN=$!
for i in $(seq 1 6000); do grep -q '"event":"\(started\|failed\)"' /tmp/o 2>/dev/null && break; sleep 0.005; done
T1=$(now)
FIRST=$(state)
echo "auto_connect: server=$FIRST since_launch_ms=$(ms $T0 $T1) first_204=$(get https://www.gstatic.com/generate_204)"
( while :; do curl -s -o /dev/null -m 1 https://www.gstatic.com/generate_204; done ) >/dev/null 2>&1 & LOOP=$!
python3 -c '
import socket,time
q=bytes.fromhex("1234010000010000000000000377777707677374617469630363 6f6d0000010001".replace(" ",""))
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
while True:
    try: s.sendto(q,("1.1.1.1",53))
    except OSError: pass
    time.sleep(0.005)
' >/dev/null 2>&1 & PROBE=$!
for round in 1 2; do
  CUR=$(state)
  A=$(addr "$CUR")
  IPS=$(getent ahostsv4 "$A" | awk '{print $1}' | sort -u | tr '\n' ',' | sed 's/,$//')
  [ -z "$IPS" ] && { echo "round $round: no address for $CUR"; break; }
  nft reset counter inet labm leaked >/dev/null
  K=$(now)
  nft add rule inet labm cut ip daddr "{ $IPS }" drop
  NEXT=""
  for i in $(seq 1 1200); do NEXT=$(state); [ -n "$NEXT" ] && [ "$NEXT" != "$CUR" ] && break; sleep 0.05; done
  S=$(now)
  echo "round $round: blocked=$CUR switched_to=$NEXT switch_ms=$(ms $K $S) leaked_packets=$(leaked) after_204=$(get https://www.gstatic.com/generate_204) guard_table=$(nft list tables | grep -c spofy_guard)"
done
ALL=$(python3 -c '
import json
for c in json.load(open("/w/sub.json")):
    s=c["outbounds"][0].get("settings",{})
    for k in ("vnext","servers"):
        if s.get(k): print(s[k][0]["address"]); break
    else:
        if s.get("address"): print(s["address"])
' | sort -u | while read -r A; do getent ahostsv4 "$A" | awk '{print $1}'; done | sort -u | tr '\n' ',' | sed 's/,$//')
nft reset counter inet labm leaked >/dev/null
K=$(now)
nft add rule inet labm cut ip daddr "{ $ALL }" drop
for i in $(seq 1 3000); do grep -q '"state":"stalled"' /tmp/o && break; sleep 0.05; done
S=$(now)
echo "round 3: all_servers_blocked state=$(grep '"event":"state"' /tmp/o | tail -1 | grep -o '"state":"[a-z]*"' | cut -d'"' -f4) after_ms=$(ms $K $S) leaked_packets=$(leaked) guard_table=$(nft list tables | grep -c spofy_guard) tun=$(ip link show spofy0 >/dev/null 2>&1 && echo present || echo GONE) proxied_site=$(get https://www.gstatic.com/generate_204 3) direct_site=$(get https://ya.ru/ 5)"
BEFORE=$(grep -c '"state":"connected"' /tmp/o)
nft flush chain inet labm cut
R=$(now)
for i in $(seq 1 1200); do [ "$(grep -c '"state":"connected"' /tmp/o)" -gt "$BEFORE" ] && break; sleep 0.05; done
C=$(now)
echo "round 3: unblocked connected_again_ms=$(ms $R $C) server=$(state) after_204=$(get https://www.gstatic.com/generate_204) leaked_packets=$(leaked)"
kill $PROBE $LOOP 2>/dev/null; pkill curl 2>/dev/null; wait $PROBE $LOOP 2>/dev/null
echo "states: $(grep '"event":"state"' /tmp/o | python3 -c '
import sys,json
prev=None; out=[]
for l in sys.stdin:
    e=json.loads(l); k=e["state"]+(":"+e["server"] if e.get("server") else "")
    if k==prev: out[-1][1]+=1
    else: out.append([k,1])
    prev=k
print(" ".join(k if n==1 else "%sx%d" % (k,n) for k,n in out))')"
kill -TERM $RUN; wait $RUN; echo "exit_code=$?"
sleep 0.3
echo "after_stop: direct=$(get https://www.gstatic.com/generate_204) guard_table=$(nft list tables | grep -c spofy_guard) tun=$(ip link show spofy0 >/dev/null 2>&1 && echo PRESENT || echo gone) processes=$(pgrep -c spofy-core) panics=$(grep -c 'panic\|fatal error' /tmp/e)"
