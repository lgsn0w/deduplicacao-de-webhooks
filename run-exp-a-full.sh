#!/usr/bin/env bash
# Full Exp A sweep: 9 cells × 5 seeds = 45 runs.
# Timeout/concurrent: 1000 events. Crash: 100 events.
set -uo pipefail  # no -e: continue past individual cell failures for robustness
cd "$(dirname "$0")"

export DATABASE_URL="postgres://harness:harness@localhost:5433/harness?sslmode=disable"
export CONSUMER_URL="http://localhost:8082"
export GOTMPDIR="$(pwd)/.tmp-go"
mkdir -p "$GOTMPDIR" results
OUTPUT="results/reproduced-exp-a.csv"
rm -f "$OUTPUT"

run_cell() {
  local strategy=$1 fault=$2 seed=$3 events=$4
  echo "$(date +%H:%M:%S) === $strategy / $fault / seed=$seed / events=$events ==="
  STRATEGY=$strategy docker compose up -d --build --force-recreate consumer 2>&1 | tail -1
  sleep 5
  go run ./cmd/loadgen \
    --strategy="$strategy" \
    --fault="$fault" \
    --seed="$seed" \
    --events="$events" \
    --output="$OUTPUT" \
    --reset=true
  sleep 10  # let TIME_WAIT connections clear between cells
}

# Phase 1: timeout + concurrent (fast — ~1 min per cell)
for seed in 1 2 3 4 5; do
  for strategy in none idem-key dedup; do
    run_cell "$strategy" timeout "$seed" 1000
    run_cell "$strategy" concurrent "$seed" 1000
  done
done

# Phase 2: crash (slow — ~20 min per cell)
for seed in 1 2 3 4 5; do
  for strategy in none idem-key dedup; do
    run_cell "$strategy" crash "$seed" 100
  done
done

# Summarize
echo "$(date +%H:%M:%S) === Summarizing ==="
go run ./cmd/summarize --input="$OUTPUT" --output=results/reproduced-exp-a-summary.csv

echo "$(date +%H:%M:%S) === Done. Results in $OUTPUT ==="
wc -l "$OUTPUT" results/reproduced-exp-a-summary.csv
