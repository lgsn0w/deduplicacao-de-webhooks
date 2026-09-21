#!/usr/bin/env bash
# Controlled transport sensitivity: 2 transports × 2 strategies × 5 seeds.
set -euo pipefail
cd "$(dirname "$0")"

export DATABASE_URL="${DATABASE_URL:-postgres://harness:harness@localhost:5433/harness?sslmode=disable}"
export CONSUMER_URL="${CONSUMER_URL:-http://localhost:8082}"
output="results/reproduced-exp-a-sensitivity.csv"
summary="results/reproduced-exp-a-sensitivity-summary.csv"
manifest="results/reproduced-exp-a-sensitivity-environment.txt"

mkdir -p results
rm -f "$output" "$summary" "$manifest"

if docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  compose=(docker-compose)
else
  echo "Docker Compose is required (docker compose or docker-compose)." >&2
  exit 1
fi

{
  echo "started_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "git_commit=$(git rev-parse HEAD 2>/dev/null || echo unavailable)"
  echo "go_version=$(go version)"
  echo "docker_version=$(docker version --format '{{.Client.Version}}' 2>/dev/null || echo unavailable)"
  echo "compose_command=${compose[*]}"
  echo "compose_version=$(${compose[@]} version --short 2>/dev/null || ${compose[@]} version 2>/dev/null || echo unavailable)"
  echo "host=$(uname -a)"
  echo "events_per_run=1000"
  echo "concurrency=10"
  echo "transports=fresh,pooled"
  echo "strategies=idem-key,dedup"
  echo "seeds=1,2,3,4,5"
} > "$manifest"

"${compose[@]}" up -d postgres

for transport in fresh pooled; do
  for strategy in idem-key dedup; do
    for seed in 1 2 3 4 5; do
      echo "$(date +%H:%M:%S) === $transport / $strategy / seed=$seed ==="
      STRATEGY="$strategy" "${compose[@]}" up -d --build --force-recreate consumer
      sleep 5
      go run ./cmd/loadgen \
        --strategy="$strategy" \
        --fault=concurrent \
        --transport="$transport" \
        --record-transport=true \
        --seed="$seed" \
        --events=1000 \
        --concurrency=10 \
        --output="$output" \
        --reset=true
      sleep 5
    done
  done
done

go run ./cmd/summarize --input="$output" --output="$summary"
{
  echo "finished_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  sha256sum "$output" "$summary"
} >> "$manifest"

wc -l "$output" "$summary"
echo "Sensitivity results: $output"
echo "Environment manifest: $manifest"
