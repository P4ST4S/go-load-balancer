#!/usr/bin/env bash
# Simple mixed load test: fire a mix of fast (/) and slow (/sleep) requests
# Usage: ./scripts/load_test_mix.sh [TOTAL_REQUESTS] [CONCURRENCY]

TOTAL=${1:-200}
CONCURRENCY=${2:-50}
LB_URL=${LB_URL:-http://localhost:3030}

echo "Running mixed load test: total=$TOTAL concurrency=$CONCURRENCY against $LB_URL"

pids=()

for i in $(seq 1 $TOTAL); do
  # Randomly choose endpoint: 70% fast, 30% slow
  if (( RANDOM % 100 < 30 )); then
    ENDPOINT="/sleep"
  else
    ENDPOINT="/"
  fi

  (
    curl -s -o /dev/null -w "%{http_code} %{time_total}s %{url_effective}\n" "$LB_URL$ENDPOINT"
  ) &
  pids+=($!)

  # throttle to control concurrency
  while [ "$(jobs -rp | wc -l)" -ge "$CONCURRENCY" ]; do
    sleep 0.01
  done
done

# wait for all
for pid in "${pids[@]}"; do
  wait "$pid" || true
done

echo "Done"
