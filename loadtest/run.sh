#!/bin/sh
# make load-test (D51, AC-77): both cache modes, demo-reset before, reconcile after.
set -eu
cd "$(dirname "$0")/.."

if ! curl -fsS localhost:8080/v1/healthz >/dev/null 2>&1; then
  echo "stack not up: docker compose up -d --build --wait"
  exit 1
fi

TABLE=loadtest/results/table.txt
mkdir -p loadtest/results
: > "$TABLE"

HEADER=$(printf '%-13s %-4s %8s %8s %8s %8s %7s %6s' scenario mode 'p50 ms' 'p95 ms' 'p99 ms' req/s errors 503s)

show() {
  echo "$HEADER"
  cat "$TABLE"
}

restore() {
  docker compose up -d --wait --no-deps api >/dev/null 2>&1 || true
}

fail() {
  echo "load-test FAILED: $1"
  show
  restore
  exit 1
}

for mode in on off; do
  echo "== CACHE_READS=$mode"
  CACHE_READS=$mode docker compose up -d --wait --no-deps api || fail "api did not start with CACHE_READS=$mode"
  docker compose exec -T -e FC_DEMO=1 api /app/admin demo-reset >/dev/null || fail "demo-reset ($mode)"
  for s in campaign cashback mixed; do
    echo "-- $mode $s"
    docker compose --profile loadtest run --rm -T -e MODE=$mode k6 run -q /scripts/$s.js >> "$TABLE" || fail "k6 $s ($mode)"
  done
  docker compose exec -T api /app/reconcile || fail "reconcile ($mode)"
done

show
restore
