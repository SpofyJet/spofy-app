set -u
get() { curl -s -o /dev/null -w '%{http_code}' -m "${2:-6}" "$1"; }
now() { date +%s%N; }
ms() { echo $(( ($2 - $1) / 1000000 )); }
export SPOFY_STATE_KEY=$(head -c 32 /dev/urandom | base64)
D=/tmp/st
once() {
  T0=$(now)
  /x/spofy-core run -sub /w/sub.json -server auto -state $D > /tmp/o 2>/tmp/e &
  RUN=$!
  for i in $(seq 1 6000); do grep -q '"event":"\(started\|failed\)"' /tmp/o 2>/dev/null && break; sleep 0.005; done
  T1=$(now)
  echo "$1: connected_since_launch_ms=$(ms $T0 $T1) server=$(grep '"state":"connected"' /tmp/o | head -1 | grep -o '"server":"[^"]*"' | cut -d'"' -f4) measured=$(grep -c '"state":"measuring"' /tmp/o) first_204=$(get https://www.gstatic.com/generate_204) $(grep -o '"event":"state_[a-z]*"\|"known":[0-9]*\|"recovered":[a-z]*\|"unreadable":[a-z]*' /tmp/o | tr -d '"' | tr '\n' ' ')"
  kill -TERM $RUN; wait $RUN
}
sum() { cat $D/state* 2>/dev/null | sha256sum | cut -c1-12; }
once first_run
once second_run
once third_run
echo "files: $(ls $D | tr '\n' ' ') modes=$(stat -c %a $D $D/state | tr '\n' ' ') bytes=$(stat -c %s $D/state)"
CLEAR=0
for W in outbounds address generate_204 $(python3 -c '
import json
for c in json.load(open("/w/sub.json")):
    s=c["outbounds"][0].get("settings",{})
    for k in ("vnext","servers"):
        if s.get(k): print(s[k][0]["address"]); break
' | sort -u | head -5); do grep -qa -- "$W" $D/state && CLEAR=$((CLEAR+1)); done
echo "words_found_in_the_clear=$CLEAR"
/x/spofy-core measure -sub /w/sub.json -state $D > /tmp/m 2>/dev/null
echo "after_measure: files=$(ls $D | tr '\n' ' ')"
python3 -c '
import sys
p=sys.argv[1]; b=bytearray(open(p,"rb").read()); b[len(b)//2]^=1; open(p,"wb").write(b)' $D/state
once damaged_main_file
BEFORE=$(sum)
GOOD=$SPOFY_STATE_KEY
export SPOFY_STATE_KEY=$(head -c 32 /dev/urandom | base64)
once wrong_key
echo "files_unchanged_by_wrong_key=$([ "$BEFORE" = "$(sum)" ] && echo yes || echo NO)"
export SPOFY_STATE_KEY=$GOOD
once right_key_again
