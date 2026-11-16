#!/bin/sh
# Run every drill ROUNDS times against the lab and keep the records.
# Usage: GW=http://node:port lab/run-all.sh results/2026-11-16
set -eu

NS="${NS:-faildrill-lab}"
ROUNDS="${ROUNDS:-3}"
out="${1:?give a directory for the records}"
: "${GW:?set GW to the gateway address, for example http://10.0.0.5:32472}"
faildrill="${FAILDRILL:-faildrill}"

mkdir -p "$out"
"$faildrill" preflight -n "$NS" > "$out/preflight.txt"

round=1
while [ "$round" -le "$ROUNDS" ]; do
  for drill in "redis-primary --fault hang" "redis-primary --fault crash" \
               "gateway-pod --fault crash" "gateway-pod --fault evict" "rolling-restart"; do
    echo "== round $round: $drill"
    # shellcheck disable=SC2086
    "$faildrill" run $drill -n "$NS" --yes --out "$out" \
      --url "open=$GW/open/get" --url "keyed=$GW/keyed/get" \
      --header "keyed=Authorization: lab-key"
    # Let Sentinel finish demoting the old primary before the next drill.
    sleep 30
  done
  round=$((round + 1))
done
