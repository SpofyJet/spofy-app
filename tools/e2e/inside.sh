set -u
SERVER=${1:-'(DE)'}
OUT=/tmp/run.out
now() { date +%s%N; }
ms() { echo $(( ($2 - $1) / 1000000 )); }
get() { curl -s -o /dev/null -w '%{http_code}' -m "${2:-8}" "$1"; }
HOSTIP=$(curl -s -m 8 https://api.ipify.org)
T0=$(now)
/x/spofy-core run -sub /w/sub.json -server "$SERVER" > $OUT 2>/tmp/run.err &
RUN=$!
for i in $(seq 1 3000); do grep -q '"event":"\(started\|failed\)"' $OUT 2>/dev/null && break; sleep 0.005; done
T1=$(now)
CODE=$(get https://www.gstatic.com/generate_204)
T2=$(now)
echo "confirmed_since_launch_ms=$(ms $T0 $T1) probe_through_tunnel_ms=$(grep '"state":"connected"' $OUT | head -1 | grep -o '"ms":[0-9]*' | cut -d: -f2) first_204=$CODE first_204_since_launch_ms=$(ms $T0 $T2)"
ENGINE=$(grep '"event":"started"' $OUT | grep -o '"pid":[0-9]*' | cut -d: -f2)
EXITIP=$(curl -s -m 8 https://api.ipify.org)
echo "host_ip_differs_from_exit=$([ -n "$EXITIP" ] && [ "$EXITIP" != "$HOSTIP" ] && echo yes || echo NO)"
echo "direct_site=$(get https://ya.ru/) fakedns_ip=$(curl -s -o /dev/null -w '%{remote_ip}' -m 8 https://www.gstatic.com/generate_204)"
echo "tcp_listeners=$(ss -ltnH | wc -l) udp_bound_wildcard=$(ss -lunH | grep -vc '%eth0')"
echo "rss_engine_kb=$(awk '/VmRSS/{print $2}' /proc/$ENGINE/status) rss_supervisor_kb=$(awk '/VmRSS/{print $2}' /proc/$RUN/status)"
for round in 1 2 3; do
  PID=$(pgrep -f 'spofy-core engine' | head -1)
  K0=$(now); kill -9 $PID
  OKAT=0
  for i in $(seq 1 200); do
    if [ "$(get https://www.gstatic.com/generate_204 2)" = 204 ] && ip link show spofy0 >/dev/null 2>&1; then OKAT=$(now); break; fi
    sleep 0.01
  done
  echo "kill_round=$round traffic_back_ms=$([ $OKAT != 0 ] && ms $K0 $OKAT || echo NEVER) supervisor_downtime_ms=$(grep -o '"downtimeMs":[0-9]*' $OUT | tail -1 | cut -d: -f2)"
done
sleep 1.5
echo "confirmed_again_after_restarts=$(( $(grep -c '"state":"connected"' $OUT) - 1 ))"
kill -USR1 $RUN; sleep 0.2; grep '"event":"traffic"' $OUT | tail -1 | cut -c1-120
kill -TERM $RUN; wait $RUN; echo "exit_code=$?"
sleep 0.2
echo "after_stop: tun=$(ip link show spofy0 >/dev/null 2>&1 && echo PRESENT || echo gone) route=$(ip route get 1.2.3.4 | head -1 | awk '{print $2,$3,$4,$5}') engine_procs=$(pgrep -fc 'spofy-core engine') direct_204=$(get https://www.gstatic.com/generate_204)"
echo "events: $(grep -o '"event":"[a-z]*"' $OUT | cut -d'"' -f4 | tr '\n' ' ')"
echo "stderr_lines=$(wc -l < /tmp/run.err) panics=$(grep -c 'panic\|fatal error' /tmp/run.err)"
sed -E 's/[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}/IP/g; s/^[0-9/]+ [0-9:.]+ //' /tmp/run.err | cut -c1-110 | sort | uniq -c | sort -rn | head -6
